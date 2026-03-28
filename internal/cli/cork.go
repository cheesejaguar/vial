package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var corkCmd = &cobra.Command{
	Use:     "cork",
	Aliases: []string{"lock"},
	Short:   "Lock the vault and clear the session",
	RunE:    runCork,
}

func init() {
	rootCmd.AddCommand(corkCmd)
}

func runCork(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	if err := session.Clear(cfg.VaultPath); err != nil {
		return fmt.Errorf("clearing session: %w", err)
	}

	fmt.Println(successMsg("🔒 Vault locked. Session cleared."))
	return nil
}
