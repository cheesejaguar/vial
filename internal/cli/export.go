package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export vault secrets to stdout",
	Long: `Export all secrets from the vault in plaintext format.

WARNING: This outputs secrets in plaintext. Pipe to a secure location only.

Requires --confirm-plaintext flag to acknowledge the risk.

Examples:
  vial export --confirm-plaintext                    # .env format to stdout
  vial export --confirm-plaintext --format json      # JSON format to stdout
  vial export --confirm-plaintext > secrets.env      # redirect to file`,
	RunE: runExport,
}

var (
	exportFormat           string
	exportConfirmPlaintext bool
)

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "env", "Output format: env or json")
	exportCmd.Flags().BoolVar(&exportConfirmPlaintext, "confirm-plaintext", false, "Acknowledge that secrets will be output in plaintext")
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

	fmt.Fprintln(os.Stderr, "⚠ WARNING: outputting secrets in plaintext")

	switch exportFormat {
	case "env":
		for _, key := range keys {
			val, err := vm.GetSecret(key)
			if err != nil {
				fmt.Fprintf(os.Stderr, "# error reading %s: %v\n", key, err)
				continue
			}
			// Quote values that contain special characters
			fmt.Printf("%s=%q\n", key, string(val.Bytes()))
			val.Destroy()
		}
	case "json":
		result := make(map[string]string, len(keys))
		for _, key := range keys {
			val, err := vm.GetSecret(key)
			if err != nil {
				continue
			}
			result[key] = string(val.Bytes())
			val.Destroy()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
	default:
		return fmt.Errorf("unknown format %q: use 'env' or 'json'", exportFormat)
	}

	return nil
}
