package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// completionCmd generates shell-specific tab-completion scripts by delegating
// to Cobra's built-in generators. The scripts are designed to be sourced once
// (persisted to the appropriate shell directory) or evaluated at shell startup
// via process substitution.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for your shell.

To load completions:

Bash:
  $ source <(vial completion bash)
  # To load completions for each session, execute once:
  $ vial completion bash > /etc/bash_completion.d/vial

Zsh:
  $ source <(vial completion zsh)
  # To load completions for each session, execute once:
  $ vial completion zsh > "${fpath[1]}/_vial"

Fish:
  $ vial completion fish | source
  # To load completions for each session, execute once:
  $ vial completion fish > ~/.config/fish/completions/vial.fish
`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Each case delegates to the corresponding Cobra generator, which
		// writes the script directly to stdout so the caller can redirect it.
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			// true = include descriptions in the generated completions.
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			// ValidArgs prevents unknown values from reaching here at
			// runtime, but the default keeps the switch exhaustive.
			return cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
