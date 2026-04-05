package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/dashboard"
	"github.com/cheesejaguar/vial/internal/project"
)

// dashboardCmd starts the embedded Svelte SPA served by a local Go HTTP
// server. The server binds exclusively to 127.0.0.1 and generates a
// single-use Bearer token that is passed to the browser via the URL fragment
// (never in a query parameter or header that could be logged by a proxy).
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the local web dashboard",
	Long:  "Start a local web server to browse your vault, manage aliases, and view project mappings.",
	RunE:  runDashboard,
}

// dashboardPort overrides the default listening port. Changing it is useful
// when running multiple vial instances concurrently (e.g. separate projects).
var dashboardPort int

func init() {
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", dashboard.DefaultPort(), "Port for the dashboard server")
	rootCmd.AddCommand(dashboardCmd)
}

// runDashboard handles the dashboard command. The vault is intentionally NOT
// re-locked via defer: the HTTP server needs the vault to remain unlocked for
// the entire lifetime of the process so it can serve API requests. The vault
// is locked implicitly when the process exits.
func runDashboard(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	// Intentionally no defer vm.Lock() — the server must keep the vault
	// unlocked while it handles requests.

	if err := loadConfig(); err != nil {
		return err
	}

	// Load the project registry so the dashboard can display per-project
	// mapping information alongside vault secrets.
	regPath := dashboard.RegistryPath(cfg.VaultPath)
	reg := project.NewRegistry(regPath)
	reg.Load() // non-fatal: an empty registry is valid

	srv, err := dashboard.NewServer(vm, reg, dashboardPort, logger, dashboard.WithConfig(cfg))
	if err != nil {
		return err
	}

	url := srv.URL()
	fmt.Println()
	fmt.Printf("%s\n", sectionHeader("🌐", "Vial Dashboard"))
	fmt.Printf("  %s %s\n", arrowIcon(), urlText(fmt.Sprintf("http://127.0.0.1:%d", dashboardPort)))
	// The token is embedded in the URL fragment so it never appears in
	// server logs, browser history referrer headers, or network captures.
	fmt.Printf("  %s\n", dimText("Session token passed via URL fragment (not logged)"))
	fmt.Println()
	fmt.Printf("  %s\n", mutedText("Press Ctrl+C to stop"))

	// Best-effort: open the browser. If this fails (headless environment,
	// unsupported OS) the user can still navigate to the printed URL.
	openBrowser(url)

	return srv.Start()
}

// openBrowser launches the system default browser pointing at url. It is
// fire-and-forget: errors are silently ignored because a browser failure
// should not prevent the server from starting.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		// Windows and other platforms are not supported; the user opens the
		// URL manually from the printed output.
		return
	}
	cmd.Start() //nolint:errcheck // failure is intentionally ignored
}
