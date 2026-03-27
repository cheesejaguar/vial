package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/awnumar/memguard"
	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/parser"
)

var distillCmd = &cobra.Command{
	Use:     "distill [FILE]",
	Aliases: []string{"import"},
	Short:   "Import keys from an existing .env file into the vault",
	Long:    "Extract secrets from an existing .env file and store them in your vault.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDistill,
}

var distillOverwrite bool

func init() {
	distillCmd.Flags().BoolVar(&distillOverwrite, "overwrite", false, "Overwrite existing vault keys without prompting")
	rootCmd.AddCommand(distillCmd)
}

func runDistill(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	envFile := ".env"
	if len(args) > 0 {
		envFile = args[0]
	}

	// Resolve relative to current dir
	if !filepath.IsAbs(envFile) {
		cwd, _ := os.Getwd()
		envFile = filepath.Join(cwd, envFile)
	}

	f, err := os.Open(envFile)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", envFile, err)
	}
	defer f.Close()

	entries, err := parser.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", envFile, err)
	}

	imported := 0
	skipped := 0
	overwritten := 0

	for _, e := range entries {
		if e.Key == "" || e.IsComment || e.IsBlank || !e.HasValue {
			continue
		}

		val := strings.TrimSpace(e.Value)
		if val == "" {
			continue
		}

		// Check if key already exists
		existing, existErr := vm.GetSecret(e.Key)
		if existErr == nil {
			existingVal := string(existing.Bytes())
			existing.Destroy()

			if existingVal == val {
				skipped++
				continue
			}

			if !distillOverwrite {
				fmt.Printf("  ⚠ %s already in vault (use --overwrite to replace)\n", e.Key)
				skipped++
				continue
			}
			overwritten++
		}

		value := memguard.NewBufferFromBytes([]byte(val))
		if err := vm.SetSecret(e.Key, value); err != nil {
			value.Destroy()
			return fmt.Errorf("storing %s: %w", e.Key, err)
		}
		value.Destroy()

		fmt.Printf("  ✓ %s distilled into vault\n", e.Key)
		imported++
	}

	fmt.Println()
	fmt.Printf("→ %d imported, %d skipped", imported, skipped)
	if overwritten > 0 {
		fmt.Printf(", %d overwritten", overwritten)
	}
	fmt.Println()

	return nil
}
