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

var (
	exportFormat           string
	exportConfirmPlaintext bool
	exportKeys             string
)

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "env", "Output format: env, json, docker-env-file, k8s-secret, github-actions, shell")
	exportCmd.Flags().BoolVar(&exportConfirmPlaintext, "confirm-plaintext", false, "Acknowledge that secrets will be output in plaintext")
	exportCmd.Flags().StringVar(&exportKeys, "keys", "", "Filter keys by glob pattern (e.g. 'STRIPE_*', 'DB_*')")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
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
	sort.Strings(keys)

	// Filter keys by glob pattern
	if exportKeys != "" {
		keys = filterKeysByGlob(keys, exportKeys)
		if len(keys) == 0 {
			return fmt.Errorf("no keys match pattern %q", exportKeys)
		}
	}

	fmt.Fprintln(os.Stderr, "⚠ WARNING: outputting secrets in plaintext")

	// Collect all key-value pairs
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
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				fmt.Printf("%s=%q\n", key, val)
			}
		}

	case "json":
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
		for _, key := range keys {
			if val, ok := secrets[key]; ok {
				fmt.Printf("%s=%s\n", key, val)
			}
		}

	case "k8s-secret":
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
			fmt.Fprintf(os.Stderr, "✓ %d secret(s) written to $GITHUB_ENV\n", len(secrets))
		}

	case "shell":
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

// filterKeysByGlob filters keys by a glob-like pattern.
// Supports * as wildcard at start or end.
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
