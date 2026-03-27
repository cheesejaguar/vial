package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var manCmd = &cobra.Command{
	Use:    "man",
	Short:  "Generate man pages",
	Hidden: true,
	RunE:   runMan,
}

var manDir string

func init() {
	manCmd.Flags().StringVar(&manDir, "dir", "./man", "Output directory for man pages")
	rootCmd.AddCommand(manCmd)
}

func runMan(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(manDir, 0755); err != nil {
		return fmt.Errorf("creating man directory: %w", err)
	}

	header := &doc.GenManHeader{
		Title:   "VIAL",
		Section: "1",
		Source:  "Vial " + version,
		Manual:  "Vial Manual",
	}

	if err := doc.GenManTree(rootCmd, header, manDir); err != nil {
		return fmt.Errorf("generating man pages: %w", err)
	}

	fmt.Printf("Man pages written to %s/\n", manDir)
	return nil
}
