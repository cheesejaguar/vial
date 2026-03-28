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

var pourCmd = &cobra.Command{
	Use:   "pour",
	Short: "Populate .env from vault using .env.example as template",
	Long:  "Read .env.example, match keys against your vault, and generate a .env file.",
	RunE:  runPour,
}

var (
	pourDryRun    bool
	pourForce     bool
	pourNoClobber bool
	pourDir       string
	pourAll       bool
)

func init() {
	pourCmd.Flags().BoolVar(&pourDryRun, "dry-run", false, "Preview matches without writing")
	pourCmd.Flags().BoolVar(&pourForce, "force", false, "Overwrite existing .env without prompting")
	pourCmd.Flags().BoolVar(&pourNoClobber, "no-clobber", false, "Keep existing .env values on conflict")
	pourCmd.Flags().StringVar(&pourDir, "dir", "", "Target project directory (default: current directory)")
	pourCmd.Flags().BoolVar(&pourAll, "all", false, "Pour all registered projects")
	rootCmd.AddCommand(pourCmd)
}

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
		fmt.Printf("\n── %s (%s)\n", p.Name, p.Path)

		// Check if .env.example exists
		templatePath := filepath.Join(p.Path, cfg.EnvExample)
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			fmt.Printf("  ⊘ No %s found, skipping\n", cfg.EnvExample)
			continue
		}

		if err := pourProject(vm, p.Path); err != nil {
			fmt.Printf("  ✗ Error: %v\n", err)
			errors++
			continue
		}
		reg.MarkPoured(p.Path)
		total++
	}

	fmt.Printf("\n→ Poured %d/%d projects", total, len(projects))
	if errors > 0 {
		fmt.Printf(" (%d errors)", errors)
	}
	fmt.Println()
	return nil
}

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

	chain := matcher.NewChain(
		&matcher.ExactMatcher{},
		&matcher.NormalizeMatcher{},
		&matcher.AliasMatcher{Store: aliasStore},
	)

	// Load existing .env values for conflict detection
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
			fmt.Printf("  ✗ %s → not found in vault\n", key)
			unmatched++
			continue
		}

		val, err := vm.GetSecret(result.VaultKey)
		if err != nil {
			logger.Warn("Could not get secret", "key", result.VaultKey, "err", err)
			unmatched++
			continue
		}
		secretValue := string(val.Bytes())
		val.Destroy()

		// Check for conflict with existing .env
		if existingVal, exists := existingValues[key]; exists && existingVal != secretValue {
			if pourNoClobber {
				resolved[key] = existingVal
				skipped++
				continue
			}

			if !pourForce {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return fmt.Errorf("conflict on %s: use --force or --no-clobber for non-interactive mode", key)
				}

				fmt.Printf("\n  ⚠ Conflict: %s\n", key)
				fmt.Printf("    Existing: %s\n", maskValue(existingVal))
				fmt.Printf("    Vault:    %s\n", maskValue(secretValue))
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
			matchInfo = fmt.Sprintf("matched (%s: %s)", result.Reason, result.VaultKey)
		}
		fmt.Printf("  ✓ %s → %s\n", key, matchInfo)
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

	if err := parser.WriteEnvFile(envPath, entries, resolved); err != nil {
		return err
	}

	total := matched + skipped
	fmt.Printf("  → .env written with %d/%d keys populated\n", total, len(keysNeeded))

	// Record audit event
	var matchedKeys []string
	for k := range resolved {
		matchedKeys = append(matchedKeys, k)
	}
	recordAudit(audit.EventPour, matchedKeys, dir, fmt.Sprintf("%d/%d keys", total, len(keysNeeded)))

	return nil
}
