package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// uncorkCmd implements the "uncork" command (alchemical name for unlock).
// It unlocks the vault and caches the derived encryption key in the system
// keyring so that subsequent commands do not require re-entering the password
// within the configured session timeout.
//
// Standard alias: "unlock"
var uncorkCmd = &cobra.Command{
	Use:     "uncork",
	Aliases: []string{"unlock"},
	Short:   "Unlock the vault with your master password",
	RunE:    runUncork,
}

func init() {
	rootCmd.AddCommand(uncorkCmd)
}

// runUncork handles the uncork command. requireUnlockedVault() performs the
// full unlock flow (session cache → VIAL_MASTER_KEY → interactive prompt) and
// stores the session in the keyring on success. The vault is re-locked via
// defer after the command exits; the keyring session persists independently
// so future commands can re-derive the DEK without user interaction.
func runUncork(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	// Lock clears the in-process DEK from mlock'd memory; the keyring session
	// is unaffected, allowing subsequent commands to unlock without a prompt.
	defer vm.Lock()

	fmt.Println(successMsg("🔓 Vault unlocked. Session cached."))
	return nil
}
