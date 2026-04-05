package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cheesejaguar/vial/internal/alias"
	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/matcher"
	"github.com/cheesejaguar/vial/internal/parser"
	"github.com/cheesejaguar/vial/internal/vault"
)

// pourCmd implements the "pour" command (alchemical name for "populate .env").
// It reads .env.example as a template, resolves each key against the vault
// using the 5-tier matcher chain, and writes the matched secrets to .env.
// Alchemical metaphor: pouring the distilled vault contents into a project's
// environment vessel (.env file).
var pourCmd = &cobra.Command{
	Use:   "pour",
	Short: "Populate .env from vault using .env.example as template",
	Long:  "Read .env.example, match keys against your vault, and generate a .env file.",
	RunE:  runPour,
}

// Pour flag state. These are package-level because Cobra registers them at
// init time and they are consumed by runPour / pourProject at call time.
var (
	// pourDryRun previews matcher results without writing any files.
	pourDryRun bool
	// pourForce suppresses the interactive conflict prompt and always takes
	// the vault value when a key exists in both .env and the vault with
	// different values.
	pourForce bool
	// pourNoClobber is the inverse of pourForce: always keep the existing
	// .env value on conflict, never overwrite with the vault value.
	pourNoClobber bool
	// pourDir overrides the working directory when targeting a specific
	// project path. Defaults to os.Getwd().
	pourDir string
	// pourAll iterates over all projects registered in the project registry
	// instead of operating on a single directory.
	pourAll bool
)

func init() {
	pourCmd.Flags().BoolVar(&pourDryRun, "dry-run", false, "Preview matches without writing")
	pourCmd.Flags().BoolVar(&pourForce, "force", false, "Overwrite existing .env without prompting")
	pourCmd.Flags().BoolVar(&pourNoClobber, "no-clobber", false, "Keep existing .env values on conflict")
	pourCmd.Flags().StringVar(&pourDir, "dir", "", "Target project directory (default: current directory)")
	pourCmd.Flags().BoolVar(&pourAll, "all", false, "Pour all registered projects")
	rootCmd.AddCommand(pourCmd)
}

// runPour is the Cobra RunE handler for the pour command. It unlocks the vault
// (via session cache, VIAL_MASTER_KEY env var, or interactive prompt), then
// delegates to runPourAll or pourProject depending on whether --all is set.
func runPour(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	if pourAll {
		return runPourAll(vm)
	}

	dir := pourDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	return pourProject(vm, dir)
}

// runPourAll iterates over every project registered in the project registry
// and calls pourProject for each one. Projects without a .env.example are
// silently skipped. After a successful pour the project is marked as poured
// in the registry so dashboards and status commands can surface freshness.
func runPourAll(vm *vault.VaultManager) error {
	reg, err := getRegistry()
	if err != nil {
		return err
	}

	projects := reg.List()
	if len(projects) == 0 {
		fmt.Println("No projects registered. Use 'vial shelf add' to register a project.")
		return nil
	}

	total := 0
	errors := 0

	for _, p := range projects {
		fmt.Printf("\n%s %s %s\n", styled(styleDim, "──"), boldText(p.Name), mutedText("("+p.Path+")"))

		// Skip projects that have not created a .env.example; we cannot know
		// which keys are needed without it.
		templatePath := filepath.Join(p.Path, cfg.EnvExample)
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			fmt.Printf("  %s No %s found, skipping\n", skipIcon(), cfg.EnvExample)
			continue
		}

		if err := pourProject(vm, p.Path); err != nil {
			fmt.Printf("  %s %s\n", errorIcon(), styled(styleError, fmt.Sprintf("Error: %v", err)))
			errors++
			continue
		}
		reg.MarkPoured(p.Path)
		total++
	}

	fmt.Printf("\n%s Poured %s/%d projects", arrowIcon(), countText(fmt.Sprintf("%d", total)), len(projects))
	if errors > 0 {
		fmt.Printf(" %s", styled(styleError, fmt.Sprintf("(%d errors)", errors)))
	}
	fmt.Println()
	return nil
}

// pourProject performs a full pour cycle for a single project directory:
//
//  1. Parse .env.example to collect the set of required keys.
//  2. Fetch all vault key names and build a 3-tier matcher chain
//     (Exact → Normalize → Alias).
//  3. Load any pre-existing .env values for conflict detection.
//  4. For each required key, attempt resolution and handle conflicts
//     according to the --force / --no-clobber / interactive flags.
//  5. Write the populated .env file via parser.WriteEnvFile (preserves
//     comments and blank lines from the template).
//  6. Record an audit event with the list of matched keys.
//
// Security note: secret values are only ever held in memory as plain strings
// for the duration of the pour operation; the LockedBuffer returned by
// GetSecret is destroyed immediately after copying into the resolved map.
func pourProject(vm *vault.VaultManager, dir string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	templatePath := filepath.Join(dir, cfg.EnvExample)
	envPath := filepath.Join(dir, ".env")

	// Parse template
	f, err := os.Open(templatePath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", cfg.EnvExample, err)
	}
	defer f.Close()

	entries, err := parser.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", cfg.EnvExample, err)
	}

	keysNeeded := parser.KeysNeeded(entries)
	if len(keysNeeded) == 0 {
		fmt.Println("  No keys found in", cfg.EnvExample)
		return nil
	}

	// Get vault keys and build matcher chain
	vaultKeys, err := vm.VaultKeyNames()
	if err != nil {
		return err
	}

	aliasStore := alias.NewStore()
	aliasStore.LoadFromVault(loadAliasStoreFromVault(vm))

	// Only tiers 1-3 are used for file-based pour; LLM and comment tiers are
	// reserved for interactive or high-latency workflows.
	chain := matcher.NewChain(
		&matcher.ExactMatcher{},
		&matcher.NormalizeMatcher{},
		&matcher.AliasMatcher{Store: aliasStore},
	)

	// Load existing .env values for conflict detection so we can ask the user
	// which value to keep when the vault and disk disagree.
	existingValues := map[string]string{}
	if data, err := os.ReadFile(envPath); err == nil {
		existingEntries, _ := parser.Parse(strings.NewReader(string(data)))
		for _, e := range existingEntries {
			if e.Key != "" && !e.IsComment && !e.IsBlank {
				existingValues[e.Key] = e.Value
			}
		}
	}

	// Resolve secrets
	resolved := map[string]string{}
	matched := 0
	unmatched := 0
	skipped := 0

	for _, key := range keysNeeded {
		result, err := chain.Resolve(key, vaultKeys)
		if err != nil {
			logger.Warn("Match error", "key", key, "err", err)
			continue
		}

		if result == nil {
			fmt.Printf("  %s %s %s not found in vault\n", errorIcon(), keyName(key), arrowIcon())
			unmatched++
			continue
		}

		val, err := vm.GetSecret(result.VaultKey)
		if err != nil {
			logger.Warn("Could not get secret", "key", result.VaultKey, "err", err)
			unmatched++
			continue
		}
		// Copy the secret value out of the locked buffer and destroy it
		// immediately to minimise the window during which plaintext lives in
		// unlocked memory.
		secretValue := string(val.Bytes())
		val.Destroy()

		// Check for conflict with existing .env
		if existingVal, exists := existingValues[key]; exists && existingVal != secretValue {
			if pourNoClobber {
				// Honour --no-clobber: keep the on-disk value unchanged.
				resolved[key] = existingVal
				skipped++
				continue
			}

			if !pourForce {
				// Non-interactive mode cannot ask the user; require an explicit
				// flag to resolve the ambiguity rather than silently picking one.
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return fmt.Errorf("conflict on %s: use --force or --no-clobber for non-interactive mode", key)
				}

				fmt.Printf("\n  %s Conflict: %s\n", warningIcon(), keyName(key))
				fmt.Printf("    %s %s\n", mutedText("Existing:"), dimText(maskValue(existingVal)))
				fmt.Printf("    %s %s\n", mutedText("Vault:   "), dimText(maskValue(secretValue)))
				fmt.Printf("    Use vault value? [Y/n]: ")

				var confirm string
				fmt.Scanln(&confirm)
				if confirm == "n" || confirm == "N" {
					resolved[key] = existingVal
					skipped++
					continue
				}
			}
		}

		matchInfo := "matched"
		if result.VaultKey != key {
			// Show the tier that produced the match so users can understand
			// why a differently-named vault key was chosen.
			matchInfo = fmt.Sprintf("matched (%s: %s)", result.Reason, result.VaultKey)
		}
		fmt.Printf("  %s %s %s %s\n", successIcon(), keyName(key), arrowIcon(), mutedText(matchInfo))
		resolved[key] = secretValue
		matched++
	}

	if pourDryRun {
		fmt.Printf("  Dry run: %d matched, %d unmatched, %d skipped\n", matched, unmatched, skipped)
		return nil
	}

	if len(resolved) == 0 && skipped == 0 {
		fmt.Println("  No keys matched. .env not created.")
		return nil
	}

	// WriteEnvFile preserves the structure (comments, blank lines, key order)
	// from the .env.example template so the output is human-readable.
	if err := parser.WriteEnvFile(envPath, entries, resolved); err != nil {
		return err
	}

	total := matched + skipped
	fmt.Printf("  %s .env written with %s/%d keys populated\n", arrowIcon(), countText(fmt.Sprintf("%d", total)), len(keysNeeded))

	// Record audit event
	var matchedKeys []string
	for k := range resolved {
		matchedKeys = append(matchedKeys, k)
	}
	recordAudit(audit.EventPour, matchedKeys, dir, fmt.Sprintf("%d/%d keys", total, len(keysNeeded)))

	return nil
}
