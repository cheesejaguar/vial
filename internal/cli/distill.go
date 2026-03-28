package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/awnumar/memguard"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/importer"
	"github.com/cheesejaguar/vial/internal/parser"
	"github.com/cheesejaguar/vial/internal/vault"
)

var distillCmd = &cobra.Command{
	Use:     "distill [FILE]",
	Aliases: []string{"import"},
	Short:   "Import keys from an existing .env file into the vault",
	Long:    "Extract secrets from an existing .env file and store them in your vault.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDistill,
}

var (
	distillOverwrite bool
	distillAll       bool
	distillFrom      string
)

func init() {
	distillCmd.Flags().BoolVar(&distillOverwrite, "overwrite", false, "Overwrite existing vault keys without prompting")
	distillCmd.Flags().BoolVar(&distillAll, "all", false, "Import all keys without interactive selection")
	distillCmd.Flags().StringVar(&distillFrom, "from", "", "Import source: 1password, doppler, vercel, json")
	rootCmd.AddCommand(distillCmd)
}

type distillCandidate struct {
	key   string
	value string
	status string // "new", "changed", "same"
}

func runDistill(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// Handle external import sources
	if distillFrom != "" {
		return runDistillFromExternal(vm, distillFrom, args)
	}

	envFile := ".env"
	if len(args) > 0 {
		envFile = args[0]
	}

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

	// Build candidate list
	var candidates []distillCandidate
	for _, e := range entries {
		if e.Key == "" || e.IsComment || e.IsBlank || !e.HasValue {
			continue
		}
		val := strings.TrimSpace(e.Value)
		if val == "" {
			continue
		}

		c := distillCandidate{key: e.Key, value: val, status: "new"}

		existing, existErr := vm.GetSecret(e.Key)
		if existErr == nil {
			existingVal := string(existing.Bytes())
			existing.Destroy()
			if existingVal == val {
				c.status = "same"
			} else {
				c.status = "changed"
			}
		}

		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		fmt.Println("No keys with values found in", filepath.Base(envFile))
		return nil
	}

	// Filter out unchanged keys
	var importable []distillCandidate
	for _, c := range candidates {
		if c.status == "same" {
			continue
		}
		if c.status == "changed" && !distillOverwrite {
			continue
		}
		importable = append(importable, c)
	}

	if len(importable) == 0 {
		sameCount := 0
		changedCount := 0
		for _, c := range candidates {
			if c.status == "same" {
				sameCount++
			} else if c.status == "changed" {
				changedCount++
			}
		}
		fmt.Printf("Nothing to import. %d already in vault", sameCount)
		if changedCount > 0 {
			fmt.Printf(", %d changed (use --overwrite)", changedCount)
		}
		fmt.Println()
		return nil
	}

	// Determine which keys to import
	var selectedKeys []string

	if distillAll || !isInteractive() {
		// Non-interactive: import all importable keys
		for _, c := range importable {
			selectedKeys = append(selectedKeys, c.key)
		}
	} else {
		// Interactive: multi-select with huh
		var options []huh.Option[string]
		for _, c := range importable {
			label := c.key
			if c.status == "changed" {
				label += " (update)"
			}
			label += "  " + maskValue(c.value)
			options = append(options, huh.NewOption(label, c.key).Selected(true))
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select keys to import").
					Description(fmt.Sprintf("Found %d key(s) in %s", len(importable), filepath.Base(envFile))).
					Options(options...).
					Value(&selectedKeys),
			),
		)

		if err := form.Run(); err != nil {
			return fmt.Errorf("selection cancelled")
		}
	}

	if len(selectedKeys) == 0 {
		fmt.Println("No keys selected.")
		return nil
	}

	// Build lookup for selected keys
	selected := make(map[string]bool)
	for _, k := range selectedKeys {
		selected[k] = true
	}

	// Import selected keys
	imported := 0
	for _, c := range importable {
		if !selected[c.key] {
			continue
		}

		value := memguard.NewBufferFromBytes([]byte(c.value))
		if err := vm.SetSecret(c.key, value); err != nil {
			value.Destroy()
			return fmt.Errorf("storing %s: %w", c.key, err)
		}
		value.Destroy()

		status := "distilled"
		if c.status == "changed" {
			status = "updated"
		}
		fmt.Printf("  ✓ %s %s\n", c.key, status)
		imported++
	}

	fmt.Printf("\n→ %d key(s) imported\n", imported)
	return nil
}

func runDistillFromExternal(vm *vault.VaultManager, source string, args []string) error {
	backend, err := importer.GetBackend(source)
	if err != nil {
		return err
	}

	if !backend.Available() {
		return fmt.Errorf("%s CLI is not installed", source)
	}

	fmt.Printf("Importing from %s...\n", source)
	secrets, err := backend.Import(args)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found.")
		return nil
	}

	fmt.Printf("Found %d secret(s)\n", len(secrets))

	imported := 0
	for _, s := range secrets {
		// Check if already exists
		existing, existErr := vm.GetSecret(s.Key)
		if existErr == nil {
			existingVal := string(existing.Bytes())
			existing.Destroy()
			if existingVal == s.Value {
				continue // same value, skip
			}
			if !distillOverwrite {
				continue // different but no overwrite
			}
		}

		value := memguard.NewBufferFromBytes([]byte(s.Value))
		if err := vm.SetSecret(s.Key, value); err != nil {
			value.Destroy()
			fmt.Printf("  ✗ %s: %v\n", s.Key, err)
			continue
		}
		value.Destroy()
		fmt.Printf("  ✓ %s imported\n", s.Key)
		imported++
	}

	fmt.Printf("\n→ %d key(s) imported from %s\n", imported, source)
	return nil
}
