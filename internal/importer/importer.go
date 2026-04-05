// Package importer provides backends for bulk-importing secrets from external
// secret stores into the vial vault.
//
// Each backend implements the [Backend] interface and wraps a specific secret
// store's native CLI tool or file format. The currently supported backends are:
//
//   - json      — reads a flat {"KEY": "value"} JSON file (no external tool needed)
//   - 1password — shells out to the 1Password CLI (op)
//   - doppler   — shells out to the Doppler CLI
//   - vercel    — shells out to the Vercel CLI
//
// Importers are selected at runtime via [GetBackend]. CLI-backed importers
// check whether their tool is in $PATH via [Backend.Available] before running,
// returning a helpful installation URL if the tool is missing.
//
// The import flow is intentionally one-directional: secrets flow from the
// external store into vial's vault. No secrets are ever written back to the
// external store by this package.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Secret is a key-value pair obtained from an external secret store. Keys
// follow the SCREAMING_SNAKE_CASE convention used by environment variables.
type Secret struct {
	Key   string // environment variable name
	Value string // plaintext secret value
}

// Backend is the common interface for all secret import sources.
// Implementations wrap a specific external store and translate its output
// into a slice of [Secret] values.
type Backend interface {
	// Name returns the short identifier for this backend (e.g. "json", "doppler").
	Name() string

	// Import fetches secrets from the external source. The args slice carries
	// backend-specific parameters (e.g. a file path for the JSON backend, a
	// vault name for 1Password, or an environment name for Vercel).
	Import(args []string) ([]Secret, error)

	// Available reports whether the underlying CLI tool is installed and
	// reachable on $PATH. It always returns true for the JSON backend, which
	// has no external dependency.
	Available() bool
}

// GetBackend returns the [Backend] registered under name, or an error
// listing the supported backend identifiers if name is unrecognised.
func GetBackend(name string) (Backend, error) {
	switch name {
	case "json":
		return &JSONImporter{}, nil
	case "1password":
		return &OnePasswordImporter{}, nil
	case "doppler":
		return &DopplerImporter{}, nil
	case "vercel":
		return &VercelImporter{}, nil
	default:
		return nil, fmt.Errorf("unknown import source: %s (supported: json, 1password, doppler, vercel)", name)
	}
}

// --- JSON Importer ---

// JSONImporter imports secrets from a flat JSON file. The expected format is:
//
//	{"KEY_NAME": "secret_value", ...}
//
// This is the simplest backend and requires no external tooling, making it
// useful for migrating from ad-hoc secret files or other tools that can export
// JSON. The file is read entirely into memory; it is the caller's
// responsibility to securely delete the source file afterwards.
type JSONImporter struct{}

// Name returns the backend identifier "json".
func (j *JSONImporter) Name() string { return "json" }

// Available always returns true because the JSON backend has no external
// dependencies — it only requires a readable file path.
func (j *JSONImporter) Available() bool { return true }

// Import reads the JSON file at args[0] and returns one [Secret] per key.
// The file must contain a JSON object whose values are all strings. Nested
// objects and arrays are not supported.
func (j *JSONImporter) Import(args []string) ([]Secret, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("json import requires a file path argument")
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("reading JSON file: %w", err)
	}

	var kvMap map[string]string
	if err := json.Unmarshal(data, &kvMap); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w (expected {\"KEY\": \"value\"} format)", err)
	}

	var secrets []Secret
	for k, v := range kvMap {
		secrets = append(secrets, Secret{Key: k, Value: v})
	}
	return secrets, nil
}

// --- 1Password Importer ---

// OnePasswordImporter imports secrets from a 1Password vault using the
// official 1Password CLI (`op`). It performs a two-step fetch:
//  1. List all items in the vault to obtain item IDs.
//  2. Retrieve each item's fields individually, filtering to those whose
//     label looks like an environment variable name (SCREAMING_SNAKE_CASE).
//
// This approach means that 1Password items are not required to follow any
// specific template — any item whose fields have env-var-style labels will
// be imported. Items with no matching fields are silently skipped.
type OnePasswordImporter struct{}

// Name returns the backend identifier "1password".
func (o *OnePasswordImporter) Name() string { return "1password" }

// Available reports whether the `op` binary is on $PATH.
func (o *OnePasswordImporter) Available() bool {
	_, err := exec.LookPath("op")
	return err == nil
}

// Import fetches secrets from 1Password. If args[0] is provided it is used
// as the vault name passed to `op item list --vault`. Otherwise the default
// vault for the currently authenticated account is used.
//
// The `op` binary must be authenticated before calling Import; if it is not,
// the underlying command will fail and the error message will hint at this.
func (o *OnePasswordImporter) Import(args []string) ([]Secret, error) {
	if !o.Available() {
		return nil, fmt.Errorf("1Password CLI (op) not found — install from https://1password.com/downloads/command-line/")
	}

	// Default: list items from the default vault and extract env var fields
	cmdArgs := []string{"item", "list", "--format=json"}
	if len(args) > 0 {
		// If a vault name is specified
		cmdArgs = append(cmdArgs, "--vault", args[0])
	}

	cmd := exec.Command("op", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("1password: %w (is 'op' authenticated?)", err)
	}

	// The item list response contains only IDs and titles; we need to fetch
	// each item individually to access its field values.
	var items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing 1password items: %w", err)
	}

	var secrets []Secret
	for _, item := range items {
		// Retrieve full field data for each item. Errors on individual items
		// are silently skipped so that one inaccessible item does not abort
		// the entire import.
		getCmd := exec.Command("op", "item", "get", item.ID, "--format=json")
		itemData, err := getCmd.Output()
		if err != nil {
			continue
		}

		var detail struct {
			Fields []struct {
				Label string `json:"label"`
				Value string `json:"value"`
				Type  string `json:"type"`
			} `json:"fields"`
		}
		if err := json.Unmarshal(itemData, &detail); err != nil {
			continue
		}

		// Only include fields whose label looks like an env var name.
		// Fields without a value (e.g. empty username placeholders) are also
		// excluded to avoid importing useless blank secrets.
		for _, field := range detail.Fields {
			if field.Value != "" && isEnvVarName(field.Label) {
				secrets = append(secrets, Secret{Key: field.Label, Value: field.Value})
			}
		}
	}

	return secrets, nil
}

// --- Doppler Importer ---

// DopplerImporter imports secrets via the Doppler CLI. It runs
// `doppler secrets download --no-file --format=json`, which outputs all
// secrets for the currently configured Doppler project/config as a flat JSON
// object. The Doppler CLI must be authenticated and have a project configured
// (via `doppler setup` or a doppler.yaml in the working directory).
type DopplerImporter struct{}

// Name returns the backend identifier "doppler".
func (d *DopplerImporter) Name() string { return "doppler" }

// Available reports whether the `doppler` binary is on $PATH.
func (d *DopplerImporter) Available() bool {
	_, err := exec.LookPath("doppler")
	return err == nil
}

// Import downloads all secrets from Doppler for the currently configured
// project. The args slice is unused; Doppler project selection is handled
// through the CLI's own configuration mechanism.
func (d *DopplerImporter) Import(args []string) ([]Secret, error) {
	if !d.Available() {
		return nil, fmt.Errorf("Doppler CLI not found — install from https://docs.doppler.com/docs/cli")
	}

	// --no-file writes the JSON directly to stdout rather than saving it as
	// a file on disk, which avoids leaving plaintext secrets in the filesystem.
	cmd := exec.Command("doppler", "secrets", "download", "--no-file", "--format=json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("doppler: %w", err)
	}

	var kvMap map[string]string
	if err := json.Unmarshal(out, &kvMap); err != nil {
		return nil, fmt.Errorf("parsing doppler output: %w", err)
	}

	var secrets []Secret
	for k, v := range kvMap {
		secrets = append(secrets, Secret{Key: k, Value: v})
	}
	return secrets, nil
}

// --- Vercel Importer ---

// VercelImporter imports environment variables from a Vercel project via the
// Vercel CLI. It runs `vercel env pull /dev/stdout` to stream the variables
// in .env format directly to stdout, avoiding a temporary file on disk.
//
// The pulled variables are scoped to a specific Vercel environment
// (development, preview, or production). If no environment is specified in
// args, "development" is used as the default.
type VercelImporter struct{}

// Name returns the backend identifier "vercel".
func (v *VercelImporter) Name() string { return "vercel" }

// Available reports whether the `vercel` binary is on $PATH.
func (v *VercelImporter) Available() bool {
	_, err := exec.LookPath("vercel")
	return err == nil
}

// Import pulls environment variables from Vercel. args[0], if provided,
// specifies the Vercel environment name ("development", "preview", or
// "production"); the default is "development".
//
// The Vercel CLI must be logged in and the current directory must be linked
// to a Vercel project (`vercel link`) before calling Import.
func (v *VercelImporter) Import(args []string) ([]Secret, error) {
	if !v.Available() {
		return nil, fmt.Errorf("Vercel CLI not found — install with: npm i -g vercel")
	}

	env := "development"
	if len(args) > 0 {
		env = args[0]
	}

	// Writing to /dev/stdout avoids creating a .env file on disk. The Vercel
	// CLI appends a trailing newline and may prepend a few comment lines, so
	// the output is parsed as a .env stream rather than consumed as JSON.
	cmd := exec.Command("vercel", "env", "pull", "/dev/stdout", "--environment", env)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("vercel: %w", err)
	}

	// Parse the .env format output. We do a minimal inline parse here rather
	// than invoking the parser package to keep the importer self-contained and
	// avoid an import cycle. Quoted values have their surrounding quotes
	// stripped; escaped characters inside quotes are not processed here because
	// Vercel's own encoding does not use backslash escapes in its output.
	var secrets []Secret
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := line[:idx]
		value := strings.Trim(line[idx+1:], "\"'")
		secrets = append(secrets, Secret{Key: key, Value: value})
	}
	return secrets, nil
}

// isEnvVarName reports whether s is a valid environment variable name under
// the POSIX convention: it must consist solely of uppercase ASCII letters,
// digits, and underscores, and must not start with a digit.
//
// This is used by the 1Password importer to filter item fields — only those
// whose labels conform to this convention are treated as environment variable
// secrets worth importing.
func isEnvVarName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 && !((c >= 'A' && c <= 'Z') || c == '_') {
			return false
		}
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}
