package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// exportCmd outputs vault secrets in plaintext to stdout in a variety of
// target formats. Because it emits raw secret values it requires an explicit
// --confirm-plaintext flag; this is a deliberate friction point to prevent
// accidental exposure (e.g. captured in shell history, CI logs).
//
// All output goes to stdout so callers can pipe it to a file or another
// process. Warnings are written to stderr so they do not corrupt the output.
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export vault secrets to stdout",
	Long: `Export all secrets from the vault in various formats.

WARNING: This outputs secrets in plaintext. Pipe to a secure location only.

Requires --confirm-plaintext flag to acknowledge the risk.

Formats:
  env              Standard .env format (default)
  json             JSON object {"KEY": "value"}
  docker-env-file  Docker --env-file compatible (unquoted KEY=VALUE)
  k8s-secret       Kubernetes Secret YAML manifest
  github-actions   Appends KEY=VALUE to $GITHUB_ENV for GitHub Actions
  shell            Shell export statements (source-able)

Examples:
  vial export --confirm-plaintext                            # .env format
  vial export --confirm-plaintext --format json              # JSON format
  vial export --confirm-plaintext --format docker-env-file   # Docker
  vial export --confirm-plaintext --format k8s-secret        # Kubernetes
  vial export --confirm-plaintext --format github-actions    # GitHub Actions
  vial export --confirm-plaintext --format shell             # Shell exports
  vial export --confirm-plaintext --keys "STRIPE_*"          # Filter keys`,
	RunE: runExport,
}

// Export flag state.
var (
	// exportFormat selects the output serialisation. Supported values:
	// "env", "json", "docker-env-file", "k8s-secret", "github-actions", "shell".
	exportFormat string
	// exportConfirmPlaintext is a required guard flag. The command refuses to
	// run without it to prevent accidental plaintext output.
	exportConfirmPlaintext bool
	// exportKeys is an optional glob pattern (e.g. "STRIPE_*") that restricts
	// the exported set to matching key names.
	exportKeys string
)

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "env", "Output format: env, json, docker-env-file, k8s-secret, github-actions, shell")
	exportCmd.Flags().BoolVar(&exportConfirmPlaintext, "confirm-plaintext", false, "Acknowledge that secrets will be output in plaintext")
	exportCmd.Flags().StringVar(&exportKeys, "keys", "", "Filter keys by glob pattern (e.g. 'STRIPE_*', 'DB_*')")
	rootCmd.AddCommand(exportCmd)
}

// runExport is the Cobra RunE handler for the export command. It enforces the
// --confirm-plaintext guard, unlocks the vault, optionally filters keys by
// glob, then dispatches to the appropriate format renderer.
//
// Security invariant: the warning banner is always written to stderr so it
// appears even when stdout is redirected to a file or pipe.
func runExport(cmd *cobra.Command, args []string) error {
	// Hard stop if the user has not explicitly acknowledged the risk. This
	// prevents `vial export` from being accidentally run in a context where
	// output is captured (CI logs, shell history, etc.).
	if !exportConfirmPlaintext {
		return fmt.Errorf("this command outputs secrets in plaintext; pass --confirm-plaintext to confirm")
	}

	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	keys, err := vm.VaultKeyNames()
	if err != nil {
		return err
	}
	// Sort keys so output is deterministic and diffable.
	sort.Strings(keys)

	// Apply optional glob filter before decrypting any values so we only
	// decrypt what is actually needed.
	if exportKeys != "" {
		keys = filterKeysByGlob(keys, exportKeys)
		if len(keys) == 0 {
			return fmt.Errorf("no keys match pattern %q", exportKeys)
		}
	}

	fmt.Fprintln(os.Stderr, warningMsg("WARNING: outputting secrets in plaintext"))

	// Decrypt all selected key values up front so the format renderers below
	// can access them without repeated vault calls. Each LockedBuffer is
	// destroyed after the string copy.
	secrets := make(map[string]string, len(keys))
	for _, key := range keys {
		val, err := vm.GetSecret(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "# error reading %s: %v\n", key, err)
			continue
		}
		secrets[key] = string(val.Bytes())
		val.Destroy()
	}

	switch exportFormat {
	case "env":
		// Standard .env format: KEY="value" with Go %q quoting so values
		// containing spaces or special characters are safe to re-parse.
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				fmt.Printf("%s=%q\n", key, val)
			}
		}

	case "json":
		// JSON object keyed by secret name. Uses json.Encoder so the output
		// is valid UTF-8 with proper escaping.
		result := make(map[string]string, len(secrets))
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				result[key] = val
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}

	case "docker-env-file":
		// Docker --env-file format: unquoted KEY=VALUE pairs. Docker reads
		// values literally without shell processing, so quoting is not needed
		// and would be included in the value.
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				fmt.Printf("%s=%s\n", key, val)
			}
		}

	case "k8s-secret":
		// Kubernetes Secret manifest (type: Opaque). Values must be base64-
		// encoded per the K8s API spec. Key names are normalised to lowercase
		// with underscores replaced by hyphens to satisfy DNS subdomain rules.
		fmt.Println("apiVersion: v1")
		fmt.Println("kind: Secret")
		fmt.Println("metadata:")
		fmt.Println("  name: vial-secrets")
		fmt.Println("  annotations:")
		fmt.Println("    generated-by: vial")
		fmt.Println("type: Opaque")
		fmt.Println("data:")
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				encoded := base64.StdEncoding.EncodeToString([]byte(val))
				fmt.Printf("  %s: %s\n", strings.ToLower(strings.ReplaceAll(key, "_", "-")), encoded)
			}
		}

	case "github-actions":
		// GitHub Actions environment file protocol: appends KEY=VALUE lines to
		// the file pointed to by $GITHUB_ENV. When not running inside GitHub
		// Actions ($GITHUB_ENV is unset) the output falls back to stdout with a
		// notice on stderr so local testing still works.
		ghEnvFile := os.Getenv("GITHUB_ENV")
		if ghEnvFile == "" {
			// Not in GitHub Actions; output to stdout
			for _, key := range keys {
				if val, ok := secrets[key]; ok {
					fmt.Printf("%s=%s\n", key, val)
				}
			}
			fmt.Fprintln(os.Stderr, "Note: $GITHUB_ENV not set; output written to stdout")
		} else {
			// Append to the runner-managed env file with 0600 permissions.
			// O_APPEND ensures we do not clobber other steps' exports.
			f, err := os.OpenFile(ghEnvFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if err != nil {
				return fmt.Errorf("opening $GITHUB_ENV: %w", err)
			}
			defer f.Close()
			for _, key := range keys {
				if val, ok := secrets[key]; ok {
					fmt.Fprintf(f, "%s=%s\n", key, val)
				}
			}
			fmt.Fprintf(os.Stderr, "%s %d secret(s) written to $GITHUB_ENV\n", successIcon(), len(secrets))
		}

	case "shell":
		// Shell export statements that can be sourced directly:
		//   source <(vial export --confirm-plaintext --format shell)
		// Values are %q-quoted so the shell handles spaces and special
		// characters correctly.
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				fmt.Printf("export %s=%q\n", key, val)
			}
		}

	default:
		return fmt.Errorf("unknown format %q: use env, json, docker-env-file, k8s-secret, github-actions, or shell", exportFormat)
	}

	return nil
}

// filterKeysByGlob returns the subset of keys that match the given glob
// pattern using filepath.Match semantics (* matches any sequence of
// non-separator characters). Unrecognised patterns are silently treated as
// non-matching rather than returning an error.
func filterKeysByGlob(keys []string, pattern string) []string {
	var filtered []string
	for _, key := range keys {
		matched, _ := filepath.Match(pattern, key)
		if matched {
			filtered = append(filtered, key)
		}
	}
	return filtered
}
