package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/hook"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git pre-commit hooks for secret leak prevention",
	Long:  "Install, uninstall, or run the vial pre-commit hook that scans staged files for leaked secrets.",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the pre-commit hook in the current git repository",
	RunE:  runHookInstall,
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the pre-commit hook from the current git repository",
	RunE:  runHookUninstall,
}

var hookCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Scan staged files for leaked vault secrets",
	Long: `Check staged git files for any values that match secrets stored in your vault.
Exits with code 1 if secrets are found. Intended for use as a pre-commit hook.

Secrets shorter than 8 characters are skipped to reduce false positives.
Create a .vialignore file to suppress specific patterns.`,
	RunE: runHookCheck,
}

var hookCheckStaged bool

func init() {
	hookCheckCmd.Flags().BoolVar(&hookCheckStaged, "staged", false, "Only check staged files (for pre-commit hook use)")
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookCheckCmd)
	rootCmd.AddCommand(hookCmd)
}

func runHookInstall(cmd *cobra.Command, args []string) error {
	dir, _ := os.Getwd()

	if err := hook.Install(dir); err != nil {
		return err
	}

	fmt.Println("✓ Pre-commit hook installed")
	fmt.Println("  Staged files will be scanned for vault secrets before each commit.")
	return nil
}

func runHookUninstall(cmd *cobra.Command, args []string) error {
	dir, _ := os.Getwd()

	if err := hook.Uninstall(dir); err != nil {
		return err
	}

	fmt.Println("✓ Pre-commit hook removed")
	return nil
}

func runHookCheck(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	dir, _ := os.Getwd()

	// Get all secret values from vault
	keys, err := vm.VaultKeyNames()
	if err != nil {
		return err
	}

	secretValues := make(map[string]string, len(keys))
	for _, key := range keys {
		val, err := vm.GetSecret(key)
		if err != nil {
			continue
		}
		secretValues[key] = string(val.Bytes())
		val.Destroy()
	}

	if len(secretValues) == 0 {
		fmt.Println("✓ No secrets in vault to check against")
		return nil
	}

	ignorePatterns := hook.LoadIgnorePatterns(dir)

	findings, err := hook.ScanStaged(dir, secretValues, ignorePatterns)
	if err != nil {
		return fmt.Errorf("scanning staged files: %w", err)
	}

	if len(findings) == 0 {
		fmt.Println("✓ No secrets found in staged files")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n✗ SECRET LEAK DETECTED: found %d secret(s) in staged files:\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "  %s:%d — contains value of %s\n", f.File, f.Line, f.KeyName)
	}
	fmt.Fprintln(os.Stderr)

	os.Exit(1)
	return nil
}
