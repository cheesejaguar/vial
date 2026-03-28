package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Secret represents a key-value pair from an external source.
type Secret struct {
	Key   string
	Value string
}

// Backend is the interface for secret importers.
type Backend interface {
	// Name returns the backend identifier.
	Name() string
	// Import fetches secrets from the external source.
	Import(args []string) ([]Secret, error)
	// Available returns true if the backend's CLI tool is installed.
	Available() bool
}

// GetBackend returns the appropriate import backend.
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

// JSONImporter imports secrets from a JSON file.
type JSONImporter struct{}

func (j *JSONImporter) Name() string    { return "json" }
func (j *JSONImporter) Available() bool { return true }

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

// OnePasswordImporter imports secrets via the 1Password CLI (op).
type OnePasswordImporter struct{}

func (o *OnePasswordImporter) Name() string { return "1password" }

func (o *OnePasswordImporter) Available() bool {
	_, err := exec.LookPath("op")
	return err == nil
}

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

	var items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing 1password items: %w", err)
	}

	var secrets []Secret
	for _, item := range items {
		// Get each item's fields
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

		for _, field := range detail.Fields {
			if field.Value != "" && isEnvVarName(field.Label) {
				secrets = append(secrets, Secret{Key: field.Label, Value: field.Value})
			}
		}
	}

	return secrets, nil
}

// --- Doppler Importer ---

// DopplerImporter imports secrets via the Doppler CLI.
type DopplerImporter struct{}

func (d *DopplerImporter) Name() string { return "doppler" }

func (d *DopplerImporter) Available() bool {
	_, err := exec.LookPath("doppler")
	return err == nil
}

func (d *DopplerImporter) Import(args []string) ([]Secret, error) {
	if !d.Available() {
		return nil, fmt.Errorf("Doppler CLI not found — install from https://docs.doppler.com/docs/cli")
	}

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

// VercelImporter imports secrets via the Vercel CLI.
type VercelImporter struct{}

func (v *VercelImporter) Name() string { return "vercel" }

func (v *VercelImporter) Available() bool {
	_, err := exec.LookPath("vercel")
	return err == nil
}

func (v *VercelImporter) Import(args []string) ([]Secret, error) {
	if !v.Available() {
		return nil, fmt.Errorf("Vercel CLI not found — install with: npm i -g vercel")
	}

	env := "development"
	if len(args) > 0 {
		env = args[0]
	}

	cmd := exec.Command("vercel", "env", "pull", "/dev/stdout", "--environment", env)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("vercel: %w", err)
	}

	// Parse the .env format output
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

// isEnvVarName checks if a string looks like an environment variable name.
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
