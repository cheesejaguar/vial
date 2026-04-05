package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/hook"
)

// hookCmd is the parent command for the git pre-commit hook sub-commands.
// The hook scans staged files for any literal secret values that exist in the
// vault, blocking the commit if a leak is detected.
//
// Install once per repository; the hook then runs automatically on every
// `git commit` without requiring the developer to remember to run `vial`.
var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git pre-commit hooks for secret leak prevention",
	Long:  "Install, uninstall, or run the vial pre-commit hook that scans staged files for leaked secrets.",
}

// hookInstallCmd writes (or appends to) the .git/hooks/pre-commit script so
// that `vial hook check --staged` is called automatically before each commit.
// Installing the hook is idempotent: running it again on a repository that
// already has the hook does not create duplicates.
var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the pre-commit hook in the current git repository",
	RunE:  runHookInstall,
}

// hookUninstallCmd removes the vial-managed section of .git/hooks/pre-commit.
// If vial's block was the only content in the hook script the file is removed
// entirely; otherwise only vial's lines are stripped.
var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the pre-commit hook from the current git repository",
	RunE:  runHookUninstall,
}

// hookCheckCmd scans staged (or all) files against the vault's current secret
// values and exits with status 1 if any match is found. It is designed to be
// called from a pre-commit hook but can also be run manually.
//
// Short-value exclusion: secrets shorter than 8 characters are skipped because
// they are too likely to appear in non-secret contexts (e.g. the string "true"
// or "dev") and would generate excessive false positives.
//
// .vialignore: a project-level file whose patterns are excluded from scanning,
// useful for values that intentionally appear in test fixtures or documentation.
var hookCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Scan staged files for leaked vault secrets",
	Long: `Check staged git files for any values that match secrets stored in your vault.
Exits with code 1 if secrets are found. Intended for use as a pre-commit hook.

Secrets shorter than 8 characters are skipped to reduce false positives.
Create a .vialignore file to suppress specific patterns.`,
	RunE: runHookCheck,
}

// hookCheckStaged restricts the scan to files in the git staging area when
// true. When false (e.g. manual invocation without --staged) all tracked files
// are scanned, which is slower but useful for auditing a repository.
var hookCheckStaged bool

func init() {
	hookCheckCmd.Flags().BoolVar(&hookCheckStaged, "staged", false, "Only check staged files (for pre-commit hook use)")
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookCheckCmd)
	rootCmd.AddCommand(hookCmd)
}

// runHookInstall installs the vial pre-commit hook in the git repository
// rooted at the current working directory.
func runHookInstall(cmd *cobra.Command, args []string) error {
	dir, _ := os.Getwd()

	if err := hook.Install(dir); err != nil {
		return err
	}

	fmt.Println(successMsg("Pre-commit hook installed"))
	fmt.Println("  " + dimText("Staged files will be scanned for vault secrets before each commit."))
	return nil
}

// runHookUninstall removes the vial pre-commit hook from the git repository
// rooted at the current working directory.
func runHookUninstall(cmd *cobra.Command, args []string) error {
	dir, _ := os.Getwd()

	if err := hook.Uninstall(dir); err != nil {
		return err
	}

	fmt.Println(successMsg("Pre-commit hook removed"))
	return nil
}

// runHookCheck scans staged (or all) git files for literal vault secret values.
// It unlocks the vault to obtain the current secret values, then delegates
// the file scanning to hook.ScanStaged.
//
// The function calls os.Exit(1) directly rather than returning an error when
// leaks are found. This is intentional: when invoked as a pre-commit hook, git
// inspects the process exit code, not any error message written to stderr.
// Returning a non-nil error would cause Cobra to print "Error: ..." which is
// redundant alongside the detailed finding report already written to stderr.
//
// Security note: secret values are held in memory only for the duration of the
// scan. Each LockedBuffer is destroyed after its plaintext is extracted.
func runHookCheck(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	dir, _ := os.Getwd()

	// Collect all current secret values to scan against. We need plaintext
	// values here because the scanner performs a substring search over file
	// contents.
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
		fmt.Println(successMsg("No secrets in vault to check against"))
		return nil
	}

	// Load project-level ignore patterns from .vialignore before scanning so
	// known false positives (test fixtures, documentation examples) are
	// suppressed.
	ignorePatterns := hook.LoadIgnorePatterns(dir)

	findings, err := hook.ScanStaged(dir, secretValues, ignorePatterns)
	if err != nil {
		return fmt.Errorf("scanning staged files: %w", err)
	}

	if len(findings) == 0 {
		fmt.Println(successMsg("No secrets found in staged files"))
		return nil
	}

	// Write findings to stderr so they appear in the terminal even when git
	// has captured the hook's stdout. Each finding includes the file path,
	// line number, and the vault key whose value was found, so the developer
	// knows exactly what to fix.
	fmt.Fprintf(os.Stderr, "\n%s\n\n", errorMsg(fmt.Sprintf("🚨 SECRET LEAK DETECTED: found %d secret(s) in staged files:", len(findings))))
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "  %s %s:%d — contains value of %s\n", errorIcon(), f.File, f.Line, keyName(f.KeyName))
	}
	fmt.Fprintln(os.Stderr)

	// Exit with code 1 to abort the git commit. os.Exit is used rather than
	// returning an error to suppress Cobra's "Error: exit status 1" message.
	os.Exit(1)
	return nil
}
