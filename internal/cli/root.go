package cli

import (
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "vial",
	Short: "The centralized secret vault for vibe coders",
	Long:  "Store your API keys once. Pour them everywhere.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/vial/config.yaml)")
}

func Execute() error {
	return rootCmd.Execute()
}
