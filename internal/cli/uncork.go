package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var uncorkCmd = &cobra.Command{
	Use:     "uncork",
	Aliases: []string{"unlock"},
	Short:   "Unlock the vault with your master password",
	RunE:    runUncork,
}

func init() {
	rootCmd.AddCommand(uncorkCmd)
}

func runUncork(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	fmt.Println(successMsg("🔓 Vault unlocked. Session cached."))
	return nil
}
