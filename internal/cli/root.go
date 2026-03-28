package cli

import (
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var cfgFile string
var verbose bool

var rootCmd = &cobra.Command{
	Use:           "vial",
	Short:         "The centralized secret vault for vibe coders",
	Long:          "Store your API keys once. Pour them everywhere.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			logger.SetLevel(log.DebugLevel)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/vial/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose/debug output")
}

func Execute() error {
	return rootCmd.Execute()
}
