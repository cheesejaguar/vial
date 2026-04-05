package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/project"
)

// shelfCmd implements the "shelf" command group (alchemical name for project
// management). A shelf holds the registered projects that can receive poured
// secrets. The project registry is persisted as projects.json alongside the
// vault file.
//
// Standard alias: "project"
var shelfCmd = &cobra.Command{
	Use:     "shelf",
	Aliases: []string{"project"},
	Short:   "Manage registered project directories",
	Long:    "Register project directories for batch pour operations.",
}

// shelfAddCmd registers a directory in the project registry. Defaults to the
// current working directory when no argument is given.
var shelfAddCmd = &cobra.Command{
	Use:   "add [DIR]",
	Short: "Register a project directory (default: current directory)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runShelfAdd,
}

// shelfListCmd prints all registered projects with their last-poured timestamp
// and whether a .env file currently exists.
var shelfListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all registered projects",
	RunE:    runShelfList,
}

// shelfRmCmd removes a project from the registry by name or path. It does not
// delete any files on disk.
var shelfRmCmd = &cobra.Command{
	Use:   "rm NAME_OR_PATH",
	Short: "Unregister a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runShelfRm,
}

func init() {
	shelfCmd.AddCommand(shelfAddCmd)
	shelfCmd.AddCommand(shelfListCmd)
	shelfCmd.AddCommand(shelfRmCmd)
	rootCmd.AddCommand(shelfCmd)
}

// getRegistry loads and returns the project registry from disk. It is a
// shared helper used by shelf sub-commands and by setup.go, which registers
// the project as part of the one-command onboarding flow.
func getRegistry() (*project.Registry, error) {
	if err := loadConfig(); err != nil {
		return nil, err
	}
	// Store projects.json in the same directory as the vault file so all
	// vial data lives together under ~/.local/share/vial/.
	regPath := filepath.Join(filepath.Dir(cfg.VaultPath), "projects.json")
	r := project.NewRegistry(regPath)
	if err := r.Load(); err != nil {
		return nil, err
	}
	return r, nil
}

// runShelfAdd handles the "shelf add" sub-command. The directory is resolved
// to an absolute path before registration so the registry always stores
// canonical paths regardless of the caller's working directory.
func runShelfAdd(cmd *cobra.Command, args []string) error {
	reg, err := getRegistry()
	if err != nil {
		return err
	}

	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	// Error is intentionally ignored: filepath.Abs only fails on Getwd errors
	// which are OS-level anomalies; callers should not rely on a relative path.
	absDir, _ := filepath.Abs(dir)

	p, err := reg.Add(absDir)
	if err != nil {
		return err
	}

	// Surface any .env* files found so the user knows what will be managed.
	envFiles := project.FindEnvFiles(absDir)
	fmt.Printf("%s Registered %s %s\n", successIcon(), boldText(p.Name), mutedText("("+p.Path+")"))
	if len(envFiles) > 0 {
		fmt.Printf("  %s Found: %v\n", arrowIcon(), envFiles)
	}
	return nil
}

// runShelfList handles the "shelf list" sub-command. A success icon is shown
// when the project already has a .env file, making it easy to spot which
// projects have been poured recently.
func runShelfList(cmd *cobra.Command, args []string) error {
	reg, err := getRegistry()
	if err != nil {
		return err
	}

	projects := reg.List()
	if len(projects) == 0 {
		fmt.Println("No projects registered. Use 'vial shelf add' to register one.")
		return nil
	}

	fmt.Printf("%s\n\n", sectionHeader("📁", "Registered Projects"))

	for _, p := range projects {
		// Default to two spaces of indent so the icon column aligns
		// with entries that do display a checkmark.
		icon := "  "
		if _, err := os.Stat(filepath.Join(p.Path, ".env")); err == nil {
			icon = successIcon() + " "
		}
		line := fmt.Sprintf("%s%s  %s", icon, boldText(p.Name), mutedText(p.Path))
		if p.LastPoured != nil {
			line += "  " + dimText("(last poured: "+p.LastPoured.Format("2006-01-02 15:04")+")")
		}
		fmt.Println(line)
	}

	fmt.Printf("\n%s\n", mutedText(fmt.Sprintf("📁 %d project(s) registered", len(projects))))
	return nil
}

// runShelfRm handles the "shelf rm" sub-command. Accepts either a project
// name (e.g. "my-app") or an absolute path.
func runShelfRm(cmd *cobra.Command, args []string) error {
	reg, err := getRegistry()
	if err != nil {
		return err
	}

	if err := reg.Remove(args[0]); err != nil {
		return err
	}

	fmt.Printf("%s Unregistered %s\n", successIcon(), boldText(args[0]))
	return nil
}
