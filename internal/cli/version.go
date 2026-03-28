package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of vial",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(banner())
		fmt.Println()
		fmt.Printf("  %s  %s\n", mutedText("version"), boldText(version))
		fmt.Printf("  %s   %s\n", mutedText("commit"), dimText(commit))
		fmt.Printf("  %s    %s\n", mutedText("built"), dimText(date))
		fmt.Println()
		fmt.Printf("  %s\n", mutedText("🧪 The centralized secret vault for vibe coders"))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
