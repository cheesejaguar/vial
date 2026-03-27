package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all secret names in the vault",
	RunE:    runList,
}

func init() {
	keyCmd.AddCommand(listCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:    "list",
		Short:  "List all secrets (alias for 'key list')",
		RunE:   runList,
		Hidden: true,
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:    "ls",
		Short:  "List all secrets (alias for 'key list')",
		RunE:   runList,
		Hidden: true,
	})
}

func runList(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	secrets := vm.ListSecrets()
	if len(secrets) == 0 {
		fmt.Println("No secrets stored. Use 'vial key set NAME' to add one.")
		return nil
	}

	// Sort by key name
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Key < secrets[j].Key
	})

	for _, s := range secrets {
		line := s.Key
		if len(s.Metadata.Tags) > 0 {
			line += fmt.Sprintf("  [%s]", joinTags(s.Metadata.Tags))
		}
		if len(s.Metadata.Aliases) > 0 {
			line += fmt.Sprintf("  (aliases: %s)", joinTags(s.Metadata.Aliases))
		}
		fmt.Println(line)
	}

	fmt.Printf("\n%d secret(s) stored\n", len(secrets))
	return nil
}

func joinTags(tags []string) string {
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += ", "
		}
		result += t
	}
	return result
}
