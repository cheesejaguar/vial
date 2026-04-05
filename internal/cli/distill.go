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

// distillCmd implements the "distill" command (alchemical name for "import").
// It reads an existing .env file (or an external secret provider) and stores
// the key-value pairs into the vault. The standard alias "import" is also
// registered but hidden to maintain the alchemical naming convention.
//
// Alchemical metaphor: distilling raw secrets from a file into purified,
// encrypted form inside the vault.
//
// Security note: secret values are never accepted as positional CLI arguments.
// They are read from the .env file at the path provided, which is the only
// way to pass plaintext secrets into the vault.
var distillCmd = &cobra.Command{
	Use:     "distill [FILE]",
	Aliases: []string{"import"},
	Short:   "Import keys from an existing .env file into the vault",
	Long:    "Extract secrets from an existing .env file and store them in your vault.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDistill,
}

// Distill flag state.
var (
	// distillOverwrite allows keys that already exist in the vault to be
	// overwritten with the value from the source file. Without this flag,
	// changed keys are reported but not imported.
	distillOverwrite bool
	// distillAll skips the interactive multi-select prompt and imports every
	// eligible key. Implied in non-interactive (non-TTY) mode.
	distillAll bool
	// distillFrom names an external backend to import from instead of a local
	// .env file. Supported values: "1password", "doppler", "vercel", "json".
	distillFrom string
)

func init() {
	distillCmd.Flags().BoolVar(&distillOverwrite, "overwrite", false, "Overwrite existing vault keys without prompting")
	distillCmd.Flags().BoolVar(&distillAll, "all", false, "Import all keys without interactive selection")
	distillCmd.Flags().StringVar(&distillFrom, "from", "", "Import source: 1password, doppler, vercel, json")
	rootCmd.AddCommand(distillCmd)
}

// distillCandidate holds a key-value pair found in the source file together
// with its import status relative to the current vault contents.
type distillCandidate struct {
	key   string
	value string
	// status is one of:
	//   "new"     — key does not exist in the vault
	//   "changed" — key exists but the value differs
	//   "same"    — key exists with an identical value (no-op)
	status string
}

// runDistill is the Cobra RunE handler for the distill command. It dispatches
// to runDistillFromExternal when --from is set, otherwise processes a local
// .env file. The vault is unlocked before any secret values are read.
func runDistill(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// External providers (1Password, Doppler, Vercel, etc.) are handled by
	// their own importer backends rather than the local file path.
	if distillFrom != "" {
		return runDistillFromExternal(vm, distillFrom, args)
	}

	envFile := ".env"
	if len(args) > 0 {
		envFile = args[0]
	}

	// Resolve relative paths against the working directory so the error
	// message always shows the full path.
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

	// Build candidate list by comparing file values to vault contents.
	// Blank values and comment-only lines are skipped; they carry no secret.
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

		// Compare against the existing vault value to classify the candidate.
		// Destroy the locked buffer immediately after reading to minimise the
		// plaintext exposure window.
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

	// Filter out unchanged keys ("same") and, unless --overwrite is set,
	// skip keys that exist with a different value ("changed").
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

	// Determine which keys to import.
	var selectedKeys []string

	if distillAll || !isInteractive() {
		// Non-interactive path: accept every eligible key automatically.
		// This is the behaviour expected in CI/CD pipelines.
		for _, c := range importable {
			selectedKeys = append(selectedKeys, c.key)
		}
	} else {
		// Interactive path: show a huh multi-select so the user can cherry-pick
		// which keys to bring into the vault. All eligible keys are pre-selected
		// so accepting the defaults is the fast path.
		var options []huh.Option[string]
		for _, c := range importable {
			label := c.key
			if c.status == "changed" {
				label += " (update)"
			}
			// Mask the value in the label so it is not visible on screen; the
			// full value is only written to the vault after the user confirms.
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

	// Build a set for O(1) membership testing when iterating importable.
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

		// Wrap the secret value in a memguard LockedBuffer so the vault
		// manager receives it in protected memory. Destroy after the call
		// regardless of success to avoid leaking plaintext on error.
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
		fmt.Printf("  %s %s %s\n", successIcon(), keyName(c.key), mutedText(status))
		imported++
	}

	fmt.Printf("\n%s %s key(s) imported\n", arrowIcon(), countText(fmt.Sprintf("%d", imported)))
	return nil
}

// runDistillFromExternal imports secrets from an external provider backend
// (e.g. 1Password, Doppler, Vercel). The backend is looked up by name via
// importer.GetBackend; if the corresponding CLI tool is not installed the
// function returns an error rather than silently producing empty output.
//
// Keys that already exist in the vault with identical values are skipped.
// Keys with differing values are skipped unless --overwrite is set.
func runDistillFromExternal(vm *vault.VaultManager, source string, args []string) error {
	backend, err := importer.GetBackend(source)
	if err != nil {
		return err
	}

	// Fail early if the external CLI tool is missing so the error is clear.
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
		// Skip secrets whose vault value is already identical to avoid
		// unnecessary writes and audit log noise.
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
			fmt.Printf("  %s %s: %v\n", errorIcon(), keyName(s.Key), err)
			continue
		}
		value.Destroy()
		fmt.Printf("  %s %s imported\n", successIcon(), keyName(s.Key))
		imported++
	}

	fmt.Printf("\n%s %s key(s) imported from %s\n", arrowIcon(), countText(fmt.Sprintf("%d", imported)), boldText(source))
	return nil
}
