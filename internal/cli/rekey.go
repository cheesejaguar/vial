package cli

import (
	"crypto/subtle"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/vault"
)

var rekeyCmd = &cobra.Command{
	Use:     "rekey",
	Aliases: []string{"change-password"},
	Short:   "Change the vault master password",
	RunE:    runRekey,
}

func init() {
	rootCmd.AddCommand(rekeyCmd)
}

func runRekey(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	vm := vault.NewVaultManager(cfg.VaultPath)

	oldPw, err := readPassword("Enter current master password: ")
	if err != nil {
		return err
	}
	defer oldPw.Destroy()

	newPw, err := readPassword("Enter new master password (min 12 chars): ")
	if err != nil {
		return err
	}
	defer newPw.Destroy()

	confirmPw, err := readPassword("Confirm new master password: ")
	if err != nil {
		return err
	}
	defer confirmPw.Destroy()

	if subtle.ConstantTimeCompare(newPw.Bytes(), confirmPw.Bytes()) != 1 {
		return fmt.Errorf("passwords do not match")
	}

	if err := vm.ChangePassword(oldPw, newPw); err != nil {
		return err
	}

	if err := session.Clear(cfg.VaultPath); err != nil {
		logger.Warn("Could not clear session cache", "err", err)
	}

	fmt.Println("Master password changed successfully.")
	return nil
}
