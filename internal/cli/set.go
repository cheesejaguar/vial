package cli

import (
	"fmt"
	"strings"

	"github.com/awnumar/memguard"
	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set NAME[=VALUE]",
	Short: "Add or update a secret in the vault",
	Long:  "Store a secret value. If VALUE is not provided inline, you will be prompted to enter it securely.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSet,
}

func init() {
	keyCmd.AddCommand(setCmd)
	// Also add as a top-level alias
	rootCmd.AddCommand(&cobra.Command{
		Use:    "set NAME[=VALUE]",
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

	arg := args[0]
	var key string
	var value *memguard.LockedBuffer

	// Check for inline KEY=VALUE format
	if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
		key = arg[:eqIdx]
		val := arg[eqIdx+1:]
		value = memguard.NewBufferFromBytes([]byte(val))
	} else {
		key = arg
		// Prompt for value
		value, err = readSecretValue(fmt.Sprintf("Enter value for %s: ", key))
		if err != nil {
			return err
		}
	}
	defer value.Destroy()

	if err := vm.SetSecret(key, value); err != nil {
		return err
	}

	fmt.Printf("✓ %s stored in vault\n", key)
	return nil
}
