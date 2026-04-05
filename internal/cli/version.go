package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd prints the build version, git commit SHA, and build timestamp.
// All three values are injected at link time by goreleaser via -ldflags into
// the package-level variables declared in root.go.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of vial",
	Run: func(cmd *cobra.Command, args []string) {
		// banner() renders the ASCII logo with lipgloss colors when stdout is
		// a TTY; it falls back to plain "vial" text for piped output.
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
