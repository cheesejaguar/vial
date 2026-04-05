// Package cli implements the Vial command-line interface using Cobra.
//
// Commands follow an alchemical naming scheme that mirrors the metaphor of a
// physical vial of liquid secrets:
//
//   - cork      — lock the vault (alias: lock)
//   - uncork    — unlock the vault (alias: unlock)
//   - pour      — inject secrets into a project's .env file (alias: run)
//   - brew      — import secrets from an existing .env file (alias: import)
//   - distill   — export secrets to a .env file (alias: export)
//   - shelf     — manage registered project directories (alias: project)
//   - label     — manage key aliases (alias: alias)
//
// Shared state flows through helpers defined in helpers.go:
//   - requireUnlockedVault() handles session cache → VIAL_MASTER_KEY → interactive prompt
//   - loadConfig() loads Viper-based YAML from ~/.config/vial/config.yaml
//   - isInteractive() guards terminal-only UI such as huh forms
//
// Styled output uses lipgloss helpers from styles.go (purple/gold theme).
// Secret values are never accepted as CLI positional arguments.
package cli

import (
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

// Build-time variables injected by goreleaser via -ldflags.
var (
	version = "dev"     // semantic version string, e.g. "1.2.3"
	commit  = "none"    // short git commit SHA
	date    = "unknown" // RFC 3339 build timestamp
)

// cfgFile holds the optional --config flag value that overrides the default
// config file path (~/.config/vial/config.yaml).
var cfgFile string

// verbose enables debug-level logging when true (set via --verbose / -v).
var verbose bool

// rootCmd is the top-level Cobra command for the "vial" binary. All
// sub-commands are registered via init() functions in their respective files.
var rootCmd = &cobra.Command{
	Use:           "vial",
	Short:         "The centralized secret vault for vibe coders",
	Long:          "Store your API keys once. Pour them everywhere.",
	SilenceUsage:  true,  // prevent Cobra from printing usage on RunE errors
	SilenceErrors: true,  // errors are printed by main, not Cobra internals
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Elevate log level early so every sub-command inherits the setting.
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

// Execute runs the root Cobra command and returns any error produced by the
// selected sub-command. It is called directly from main().
func Execute() error {
	return rootCmd.Execute()
}
