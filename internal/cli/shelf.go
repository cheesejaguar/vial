package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/project"
)

var shelfCmd = &cobra.Command{
	Use:     "shelf",
	Aliases: []string{"project"},
	Short:   "Manage registered project directories",
	Long:    "Register project directories for batch pour operations.",
}

var shelfAddCmd = &cobra.Command{
	Use:   "add [DIR]",
	Short: "Register a project directory (default: current directory)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runShelfAdd,
}

var shelfListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all registered projects",
	RunE:    runShelfList,
}

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

func getRegistry() (*project.Registry, error) {
	if err := loadConfig(); err != nil {
		return nil, err
	}
	regPath := filepath.Join(filepath.Dir(cfg.VaultPath), "projects.json")
	r := project.NewRegistry(regPath)
	if err := r.Load(); err != nil {
		return nil, err
	}
	return r, nil
}

func runShelfAdd(cmd *cobra.Command, args []string) error {
	reg, err := getRegistry()
	if err != nil {
		return err
	}

	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, _ := filepath.Abs(dir)

	p, err := reg.Add(absDir)
	if err != nil {
		return err
	}

	envFiles := project.FindEnvFiles(absDir)
	fmt.Printf("%s Registered %s %s\n", successIcon(), boldText(p.Name), mutedText("("+p.Path+")"))
	if len(envFiles) > 0 {
		fmt.Printf("  %s Found: %v\n", arrowIcon(), envFiles)
	}
	return nil
}

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
