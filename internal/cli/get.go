package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get NAME",
	Short: "Retrieve a secret from the vault",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func init() {
	keyCmd.AddCommand(getCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:    "get NAME",
		Short:  "Retrieve a secret (alias for 'key get')",
		Args:   cobra.ExactArgs(1),
		RunE:   runGet,
		Hidden: true,
	})
}

func runGet(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	value, err := vm.GetSecret(args[0])
	if err != nil {
		return err
	}
	defer value.Destroy()

	fmt.Print(string(value.Bytes()))
	return nil
}
