package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// corkCmd implements the "cork" command (alchemical name for lock).
// It removes the cached DEK from the system keyring so that the vault cannot
// be unlocked without the master password until a new session is established
// via "vial uncork".
//
// Standard alias: "lock"
var corkCmd = &cobra.Command{
	Use:     "cork",
	Aliases: []string{"lock"},
	Short:   "Lock the vault and clear the session",
	RunE:    runCork,
}

func init() {
	rootCmd.AddCommand(corkCmd)
}

// runCork handles the cork command. It does not need an unlocked vault — it
// only needs to remove the keyring entry that stores the cached DEK. The
// vault file itself is always encrypted at rest; clearing the session simply
// forces re-authentication on the next vault-accessing command.
func runCork(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	// Clear the DEK cached in the system keyring for this vault path.
	// After this call, requireUnlockedVault() will prompt for the master
	// password (or read VIAL_MASTER_KEY) on the next command.
	if err := session.Clear(cfg.VaultPath); err != nil {
		return fmt.Errorf("clearing session: %w", err)
	}

	fmt.Println(successMsg("🔒 Vault locked. Session cleared."))
	return nil
}
