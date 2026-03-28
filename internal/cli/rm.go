package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm NAME",
	Aliases: []string{"remove", "delete"},
	Short:   "Remove a secret from the vault",
	Args:    cobra.ExactArgs(1),
	RunE:    runRm,
}

var forceRm bool

func init() {
	rmCmd.Flags().BoolVarP(&forceRm, "force", "f", false, "Skip confirmation prompt")
	keyCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:    "rm NAME",
		Short:  "Remove a secret (alias for 'key rm')",
		Args:   cobra.ExactArgs(1),
		RunE:   runRm,
		Hidden: true,
	})
}

func runRm(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	key := args[0]

	if !forceRm {
		fmt.Printf("Remove %q from vault? [y/N]: ", key)
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println(mutedText("Cancelled."))
			return nil
		}
	}

	if err := vm.RemoveSecret(key); err != nil {
		return err
	}

	fmt.Printf("%s %s removed from vault\n", successIcon(), keyName(key))
	return nil
}
