//go:build playwright

package dashboard

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"io/fs"

	"github.com/awnumar/memguard"
	"github.com/charmbracelet/log"

	"github.com/cheesejaguar/vial/internal/vault"
)

func TestDashboardPlaywright(t *testing.T) {
	// Create test vault
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	vm := vault.NewVaultManager(vaultPath)
	vm.SetKDFParams(vault.TestKDFParams())

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()
	if err := vm.Init(password); err != nil {
		t.Fatalf("Init vault: %v", err)
	}

	// Store a test secret
	val := memguard.NewBufferFromBytes([]byte("sk-test-123"))
	if err := vm.SetSecret("TEST_KEY", val); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	val.Destroy()

	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create server
	logger := log.NewWithOptions(os.Stderr, log.Options{Level: log.FatalLevel})
	srv, err := NewServer(vm, nil, port, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Build the mux manually (same as Start but without blocking)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/lock", srv.authMiddleware(srv.handleLock))
	mux.HandleFunc("GET /api/auth/status", srv.authMiddleware(srv.handleAuthStatus))
	mux.HandleFunc("GET /api/vault/secrets", srv.authMiddleware(srv.handleListSecrets))
	mux.HandleFunc("GET /api/vault/secrets/{key}", srv.authMiddleware(srv.handleGetSecret))
	mux.HandleFunc("DELETE /api/vault/secrets/{key}", srv.authMiddleware(srv.handleDeleteSecret))
	mux.HandleFunc("GET /api/aliases", srv.authMiddleware(srv.handleListAliases))
	mux.HandleFunc("GET /api/projects", srv.authMiddleware(srv.handleListProjects))
	mux.HandleFunc("GET /api/health/overview", srv.authMiddleware(srv.handleHealthOverview))

	staticFS, err := fs.Sub(frontendFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, staticFS)
	})

	handler := srv.corsHostMiddleware(mux)

	// Start HTTP server in background
	httpServer := &http.Server{Handler: handler}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go httpServer.Serve(ln)
	defer httpServer.Close()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Run Playwright test
	cmd := exec.Command("node", "/tmp/playwright_dashboard_test.js", fmt.Sprintf("%d", port), srv.Token())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Playwright test failed: %v", err)
	}
}
