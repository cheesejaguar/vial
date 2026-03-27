package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/vault"
)

var labelCmd = &cobra.Command{
	Use:     "label",
	Aliases: []string{"alias"},
	Short:   "Manage aliases and tags for stored keys",
}

var labelSetCmd = &cobra.Command{
	Use:   "set ALIAS=CANONICAL",
	Short: "Map an alias name to a canonical vault key",
	Long:  "Example: vial label set OPENAI_KEY=OPENAI_API_KEY",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelSet,
}

var labelListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all aliases",
	RunE:    runLabelList,
}

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

func runLabelSet(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	parts := strings.SplitN(args[0], "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("usage: vial label set ALIAS=CANONICAL_KEY")
	}

	aliasName := strings.TrimSpace(parts[0])
	canonicalKey := strings.TrimSpace(parts[1])

	// Verify canonical key exists
	meta, err := vm.GetMetadata(canonicalKey)
	if err != nil {
		return fmt.Errorf("key %q not found in vault", canonicalKey)
	}

	// Add alias to metadata
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

	fmt.Printf("✓ %s → %s\n", aliasName, canonicalKey)
	return nil
}

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
				fmt.Printf("  %s → %s\n", a, s.Key)
			}
			found = true
		}
	}

	if !found {
		fmt.Println("No aliases defined. Use 'vial label set ALIAS=KEY' to create one.")
	}

	return nil
}

func runLabelRm(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	aliasName := args[0]

	// Find which key has this alias
	secrets := vm.ListSecrets()
	for _, s := range secrets {
		for i, a := range s.Metadata.Aliases {
			if a == aliasName {
				meta, err := vm.GetMetadata(s.Key)
				if err != nil {
					return err
				}
				meta.Aliases = append(meta.Aliases[:i], meta.Aliases[i+1:]...)
				if err := vm.SetMetadata(s.Key, *meta); err != nil {
					return err
				}
				fmt.Printf("✓ Removed alias %s from %s\n", aliasName, s.Key)
				return nil
			}
		}
	}

	return fmt.Errorf("alias %q not found", aliasName)
}

// loadAliasStoreFromVault loads all aliases from vault metadata into an alias store.
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
