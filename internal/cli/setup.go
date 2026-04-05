package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/hook"
	"github.com/cheesejaguar/vial/internal/scanner"
)

// setupCmd implements a zero-config project onboarding workflow that chains
// five discrete steps: source-code scanning, .env.example generation, shelf
// registration, secret pouring, and git hook installation. Each step is
// attempted independently so a failure in one step does not abort the others;
// errors are printed inline with a skip icon and setup continues.
var setupCmd = &cobra.Command{
	Use:   "setup [DIR]",
	Short: "One-command project onboarding",
	Long: `Zero-config project setup: scan source code, generate .env.example,
register in shelf, pour secrets, and install git hooks — all in one command.

Steps performed:
  1. Scan source code for env var references
  2. Generate .env.example if missing
  3. Register project in shelf
  4. Pour secrets from vault
  5. Install git pre-commit hook (if .git/ exists)

Examples:
  vial setup                 # set up current directory
  vial setup ./my-project    # set up specific project
  vial setup --yes           # non-interactive mode`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSetup,
}

// setupYes suppresses interactive confirmation prompts when true, accepting
// all defaults automatically. Intended for scripted or CI usage.
var setupYes bool

func init() {
	setupCmd.Flags().BoolVar(&setupYes, "yes", false, "Non-interactive mode: accept all defaults")
	rootCmd.AddCommand(setupCmd)
}

// runSetup orchestrates the five-step onboarding flow. Steps are numbered
// with circled Unicode digits via stepNumber() so the user can track progress
// at a glance. Steps that are not applicable (e.g. no .git directory) are
// skipped with a visible explanation rather than silently omitted.
func runSetup(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if err := loadConfig(); err != nil {
		return err
	}

	fmt.Printf("%s\n\n", sectionHeader("🧪", fmt.Sprintf("Setting up project: %s", mutedText(absDir))))

	// Step 1: Scan source code for process.env / os.Getenv / etc. calls.
	// The result is used downstream by step 2 to populate .env.example.
	fmt.Printf("%s Scanning source code for env var references...\n", stepNumber(1))
	result, err := scanner.ScanDir(absDir)
	if err != nil {
		fmt.Printf("  %s Scan failed: %v\n", skipIcon(), err)
	} else if len(result.Refs) > 0 {
		fmt.Printf("  %s\n", result.Summary())
	} else {
		fmt.Println("  No env var references found in source code.")
	}

	// Step 2: Generate .env.example only if it does not already exist.
	// An existing file is respected as the source of truth — it may contain
	// hand-written comments or additional keys not found by the scanner.
	templatePath := filepath.Join(absDir, cfg.EnvExample)
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		if result != nil && len(result.Refs) > 0 {
			fmt.Printf("\n%s Generating %s...\n", stepNumber(2), cfg.EnvExample)
			if err := runScaffoldForDir(absDir, result); err != nil {
				fmt.Printf("  %s Failed: %v\n", skipIcon(), err)
			} else {
				fmt.Printf("  %s Generated %s\n", successIcon(), cfg.EnvExample)
			}
		} else {
			fmt.Printf("\n%s No env vars found; skipping %s generation\n", stepNumber(2), cfg.EnvExample)
		}
	} else {
		fmt.Printf("\n%s %s already exists\n", stepNumber(2), cfg.EnvExample)
	}

	// Step 3: Register the project in the shelf (project registry) so that
	// "vial pour --all" and batch operations include it automatically.
	fmt.Printf("\n%s Registering project in shelf...\n", stepNumber(3))
	reg, err := getRegistry()
	if err != nil {
		fmt.Printf("  %s Registry error: %v\n", skipIcon(), err)
	} else {
		p, err := reg.Add(absDir)
		if err != nil {
			// reg.Add returns an error when the project is already registered;
			// treat this as a non-fatal skip rather than a hard failure.
			fmt.Printf("  %s Already registered or error: %v\n", skipIcon(), err)
		} else {
			fmt.Printf("  %s Registered %s\n", successIcon(), boldText(p.Name))
		}
	}

	// Step 4: Pour secrets — only if .env.example exists to drive the
	// 5-tier matcher. If this step is reached the vault prompt may appear.
	if _, err := os.Stat(templatePath); err == nil {
		fmt.Printf("\n%s Pouring secrets from vault...\n", stepNumber(4))
		vm, err := requireUnlockedVault()
		if err != nil {
			fmt.Printf("  %s Vault error: %v\n", skipIcon(), err)
		} else {
			if err := pourProject(vm, absDir); err != nil {
				fmt.Printf("  %s Pour error: %v\n", skipIcon(), err)
			}
			// Lock explicitly rather than via defer so the vault DEK is
			// cleared before we proceed to step 5.
			vm.Lock()
		}
	} else {
		fmt.Printf("\n%s No %s found; skipping pour\n", stepNumber(4), cfg.EnvExample)
	}

	// Step 5: Install the git pre-commit hook that warns when a .env file
	// would be committed. Only relevant if the directory is a git repository.
	gitDir := filepath.Join(absDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if !hook.IsInstalled(absDir) {
			fmt.Printf("\n%s Installing git pre-commit hook...\n", stepNumber(5))
			if err := hook.Install(absDir); err != nil {
				fmt.Printf("  %s Hook error: %v\n", skipIcon(), err)
			} else {
				fmt.Printf("  %s Pre-commit hook installed\n", successIcon())
			}
		} else {
			fmt.Printf("\n%s Git hook already installed\n", stepNumber(5))
		}
	} else {
		fmt.Printf("\n%s No .git directory; skipping hook installation\n", stepNumber(5))
	}

	fmt.Printf("\n%s\n", successMsg("✨ Project setup complete!"))
	return nil
}

// runScaffoldForDir generates a minimal .env.example file from the variable
// names discovered by the scanner. The file is written with mode 0600 to
// reduce the risk of accidentally committing secrets if a developer fills in
// real values before adding the file to .gitignore.
func runScaffoldForDir(absDir string, result *scanner.ScanResult) error {
	varNames := result.UniqueVarNames()
	if len(varNames) == 0 {
		return nil
	}

	outputPath := filepath.Join(absDir, cfg.EnvExample)

	var lines []string
	lines = append(lines, "# Environment variables for "+filepath.Base(absDir))
	lines = append(lines, "# Generated by: vial setup")
	lines = append(lines, "")

	for i, name := range varNames {
		lines = append(lines, name+"=")
		// Blank line between entries for readability; omit after the last one.
		if i < len(varNames)-1 {
			lines = append(lines, "")
		}
	}

	content := ""
	for _, line := range lines {
		content += line + "\n"
	}

	return os.WriteFile(outputPath, []byte(content), 0600)
}
