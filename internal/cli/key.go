package cli

import (
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage secrets in the vault",
	Long:  "Add, retrieve, list, and remove secrets from the vault.",
}

func init() {
	rootCmd.AddCommand(keyCmd)
}
