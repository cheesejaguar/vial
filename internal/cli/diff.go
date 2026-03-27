package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/alias"
	"github.com/cheesejaguar/vial/internal/matcher"
	"github.com/cheesejaguar/vial/internal/parser"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show what .env.example needs vs what the vault has",
	RunE:  runDiff,
}

var diffDir string

func init() {
	diffCmd.Flags().StringVar(&diffDir, "dir", "", "Target project directory")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	dir := diffDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

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
		return err
	}

	keysNeeded := parser.KeysNeeded(entries)
	if len(keysNeeded) == 0 {
		fmt.Println("No keys found in", cfg.EnvExample)
		return nil
	}

	// Get vault keys and build matcher
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

	// Load existing .env
	existingValues := map[string]string{}
	if data, err := os.ReadFile(envPath); err == nil {
		existingEntries, _ := parser.Parse(strings.NewReader(string(data)))
		for _, e := range existingEntries {
			if e.Key != "" && !e.IsComment && !e.IsBlank {
				existingValues[e.Key] = e.Value
			}
		}
	}

	// Compare
	matched := 0
	missing := 0
	stale := 0

	for _, key := range keysNeeded {
		result, _ := chain.Resolve(key, vaultKeys)
		existingVal, hasExisting := existingValues[key]

		switch {
		case result == nil:
			fmt.Printf("  ✗ %s — not in vault\n", key)
			missing++
		case !hasExisting:
			fmt.Printf("  + %s — in vault, not in .env\n", key)
			matched++
		default:
			// Check if .env value matches vault
			vaultVal, err := vm.GetSecret(result.VaultKey)
			if err != nil {
				fmt.Printf("  ? %s — error reading vault\n", key)
				continue
			}
			vaultValStr := string(vaultVal.Bytes())
			vaultVal.Destroy()

			if existingVal == vaultValStr {
				fmt.Printf("  ✓ %s — up to date\n", key)
			} else {
				fmt.Printf("  ⚠ %s — .env differs from vault\n", key)
				stale++
			}
			matched++
		}
	}

	fmt.Println()
	fmt.Printf("→ %d matched, %d missing from vault", matched, missing)
	if stale > 0 {
		fmt.Printf(", %d stale", stale)
	}
	fmt.Println()
	return nil
}
