package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/vault"
)

// labelCmd implements the "label" command group (alchemical name for alias
// management). Labels let users map alternate names — such as OPENAI_KEY or
// NEXT_PUBLIC_OPENAI_KEY — to a single canonical vault key. The matcher.Chain
// (Tier 3 - Alias) consults these labels during pour operations so that
// projects using non-standard env var names still receive the correct secrets.
//
// Standard alias: "alias"
var labelCmd = &cobra.Command{
	Use:     "label",
	Aliases: []string{"alias"},
	Short:   "Manage aliases and tags for stored keys",
}

// labelSetCmd creates or replaces a mapping from ALIAS to a canonical key.
// The argument uses "=" as a delimiter (e.g. OPENAI_KEY=OPENAI_API_KEY) so
// that both names remain legible at a glance and shell quoting is unnecessary.
var labelSetCmd = &cobra.Command{
	Use:   "set ALIAS=CANONICAL",
	Short: "Map an alias name to a canonical vault key",
	Long:  "Example: vial label set OPENAI_KEY=OPENAI_API_KEY",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelSet,
}

// labelListCmd prints every alias defined across all vault keys, formatted as
// "alias → CANONICAL_KEY" to make the direction of the mapping explicit.
var labelListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all aliases",
	RunE:    runLabelList,
}

// labelRmCmd removes a single alias from whichever key currently owns it.
var labelRmCmd = &cobra.Command{
	Use:   "rm ALIAS",
	Short: "Remove an alias from a vault key",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelRm,
}

func init() {
	labelCmd.AddCommand(labelSetCmd)
	labelCmd.AddCommand(labelListCmd)
	labelCmd.AddCommand(labelRmCmd)
	rootCmd.AddCommand(labelCmd)
}

// runLabelSet handles the "label set" sub-command. Aliases are stored inside
// the canonical key's metadata in the vault file, so the vault must be
// unlocked even though we are only updating metadata (not secret values).
func runLabelSet(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// Split on the first "=" only so canonical key names that contain "="
	// (unusual but possible) are preserved correctly.
	parts := strings.SplitN(args[0], "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("usage: vial label set ALIAS=CANONICAL_KEY")
	}

	aliasName := strings.TrimSpace(parts[0])
	canonicalKey := strings.TrimSpace(parts[1])

	// Verify canonical key exists before writing, to give a clear error
	// rather than silently creating a dangling alias.
	meta, err := vm.GetMetadata(canonicalKey)
	if err != nil {
		return fmt.Errorf("key %q not found in vault", canonicalKey)
	}

	// Idempotency: skip if the alias is already registered on this key.
	for _, existing := range meta.Aliases {
		if existing == aliasName {
			fmt.Printf("Alias %s → %s already exists\n", aliasName, canonicalKey)
			return nil
		}
	}

	meta.Aliases = append(meta.Aliases, aliasName)
	if err := vm.SetMetadata(canonicalKey, *meta); err != nil {
		return err
	}

	fmt.Printf("%s %s %s %s\n", successIcon(), keyName(aliasName), arrowIcon(), keyName(canonicalKey))
	return nil
}

// runLabelList handles the "label list" sub-command. It iterates over all
// secrets and prints aliases for any key that has at least one defined.
func runLabelList(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	secrets := vm.ListSecrets()
	found := false

	for _, s := range secrets {
		if len(s.Metadata.Aliases) > 0 {
			for _, a := range s.Metadata.Aliases {
				fmt.Printf("  %s %s %s\n", mutedText(a), arrowIcon(), keyName(s.Key))
			}
			found = true
		}
	}

	if !found {
		fmt.Println("No aliases defined. Use 'vial label set ALIAS=KEY' to create one.")
	}

	return nil
}

// runLabelRm handles the "label rm" sub-command. It performs a linear scan of
// all vault secrets to locate which key owns the alias, then removes it by
// index from the slice. The vault is re-written after the update.
func runLabelRm(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	aliasName := args[0]

	// Aliases live on arbitrary keys, so we must scan all secrets to find
	// which canonical key currently owns the requested alias name.
	secrets := vm.ListSecrets()
	for _, s := range secrets {
		for i, a := range s.Metadata.Aliases {
			if a == aliasName {
				meta, err := vm.GetMetadata(s.Key)
				if err != nil {
					return err
				}
				// Remove by index while preserving the order of remaining aliases.
				meta.Aliases = append(meta.Aliases[:i], meta.Aliases[i+1:]...)
				if err := vm.SetMetadata(s.Key, *meta); err != nil {
					return err
				}
				fmt.Printf("%s Removed alias %s from %s\n", successIcon(), keyName(aliasName), keyName(s.Key))
				return nil
			}
		}
	}

	return fmt.Errorf("alias %q not found", aliasName)
}

// loadAliasStoreFromVault builds a canonical-key → []alias-names map from
// vault metadata. It is used by pour.go and brew.go to seed the Tier 3
// (Alias) matcher with user-defined labels before running the match chain.
func loadAliasStoreFromVault(vm *vault.VaultManager) map[string][]string {
	secrets := vm.ListSecrets()
	keyAliases := make(map[string][]string)
	for _, s := range secrets {
		if len(s.Metadata.Aliases) > 0 {
			keyAliases[s.Key] = s.Metadata.Aliases
		}
	}
	return keyAliases
}
