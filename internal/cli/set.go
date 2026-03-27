package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set NAME",
	Short: "Add or update a secret in the vault",
	Long:  "Store a secret value. You will be prompted to enter it securely, or you can pipe it via stdin.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSet,
}

func init() {
	keyCmd.AddCommand(setCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:    "set NAME",
		Short:  "Add or update a secret (alias for 'key set')",
		Args:   cobra.ExactArgs(1),
		RunE:   runSet,
		Hidden: true,
	})
}

func runSet(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	key := args[0]

	value, err := readSecretValue(fmt.Sprintf("Enter value for %s: ", key))
	if err != nil {
		return err
	}
	defer value.Destroy()

	if err := vm.SetSecret(key, value); err != nil {
		return err
	}

	fmt.Printf("✓ %s stored in vault\n", key)
	return nil
}
