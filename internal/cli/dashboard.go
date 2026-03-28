package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/dashboard"
	"github.com/cheesejaguar/vial/internal/project"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the local web dashboard",
	Long:  "Start a local web server to browse your vault, manage aliases, and view project mappings.",
	RunE:  runDashboard,
}

var dashboardPort int

func init() {
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", dashboard.DefaultPort(), "Port for the dashboard server")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	// Don't defer vm.Lock() — we need it to stay unlocked while the server runs

	if err := loadConfig(); err != nil {
		return err
	}

	// Load project registry
	regPath := dashboard.RegistryPath(cfg.VaultPath)
	reg := project.NewRegistry(regPath)
	reg.Load()

	srv, err := dashboard.NewServer(vm, reg, dashboardPort, logger, dashboard.WithConfig(cfg))
	if err != nil {
		return err
	}

	url := srv.URL()
	fmt.Println()
	fmt.Printf("%s\n", sectionHeader("🌐", "Vial Dashboard"))
	fmt.Printf("  %s %s\n", arrowIcon(), urlText(fmt.Sprintf("http://127.0.0.1:%d", dashboardPort)))
	fmt.Printf("  %s\n", dimText("Session token passed via URL fragment (not logged)"))
	fmt.Println()
	fmt.Printf("  %s\n", mutedText("Press Ctrl+C to stop"))

	// Open browser
	openBrowser(url)

	return srv.Start()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Start()
}
