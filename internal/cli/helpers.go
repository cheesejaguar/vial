package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/awnumar/memguard"
	"github.com/charmbracelet/log"
	"golang.org/x/term"

	"github.com/cheesejaguar/vial/internal/config"
	"github.com/cheesejaguar/vial/internal/keyring"
	"github.com/cheesejaguar/vial/internal/vault"
)

var (
	logger  = log.NewWithOptions(os.Stderr, log.Options{})
	cfg     *config.Config
	session = keyring.NewSessionManager()
)

// loadConfig loads the application config.
func loadConfig() error {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return nil
}

// openVault creates a vault manager and attempts to unlock via session cache.
// If no session exists, it returns the locked vault manager.
func openVault() (*vault.VaultManager, error) {
	if err := loadConfig(); err != nil {
		return nil, err
	}

	vm := vault.NewVaultManager(cfg.VaultPath)

	// Try session cache first
	dekBytes, err := session.Retrieve(cfg.VaultPath)
	if err == nil {
		if err := vm.UnlockWithDEK(dekBytes); err == nil {
			return vm, nil
		}
		// Session DEK is invalid, clear it
		session.Clear(cfg.VaultPath)
	}

	return vm, nil
}

// requireUnlockedVault returns an unlocked vault, prompting for password if needed.
// In CI/CD mode, reads password from VIAL_MASTER_KEY env var.
func requireUnlockedVault() (*vault.VaultManager, error) {
	vm, err := openVault()
	if err != nil {
		if errors.Is(err, vault.ErrVaultNotFound) {
			return nil, fmt.Errorf("no vault found — run 'vial init' to create one")
		}
		return nil, err
	}

	if vm.IsUnlocked() {
		return vm, nil
	}

	// Try VIAL_MASTER_KEY env var (CI/CD headless mode)
	if masterKey := os.Getenv("VIAL_MASTER_KEY"); masterKey != "" {
		password := memguard.NewBufferFromBytes([]byte(masterKey))
		defer password.Destroy()

		if err := vm.Unlock(password); err != nil {
			if errors.Is(err, vault.ErrVaultNotFound) {
				return nil, fmt.Errorf("no vault found at %s — run 'vial init' to create one", cfg.VaultPath)
			}
			return nil, fmt.Errorf("VIAL_MASTER_KEY unlock failed: %w", err)
		}

		logger.Debug("Unlocked via VIAL_MASTER_KEY")
		return vm, nil
	}

	// Interactive: prompt for password
	if !isInteractive() {
		return nil, fmt.Errorf("vault is locked and no VIAL_MASTER_KEY set; cannot prompt in non-interactive mode")
	}

	password, err := readPassword("Enter master password: ")
	if err != nil {
		return nil, err
	}
	defer password.Destroy()

	if err := vm.Unlock(password); err != nil {
		if errors.Is(err, vault.ErrVaultNotFound) {
			return nil, fmt.Errorf("no vault found at %s — run 'vial init' to create one", cfg.VaultPath)
		}
		return nil, err
	}

	// Cache session
	if dekBytes := vm.DEKBytes(); dekBytes != nil {
		if err := session.Store(cfg.VaultPath, dekBytes, cfg.SessionTimeout); err != nil {
			logger.Warn("Could not cache session in keyring", "err", err)
		}
	}

	return vm, nil
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// readPassword reads a password from the terminal with hidden input.
// Falls back to reading from stdin if not a terminal (piped input).
func readPassword(prompt string) (*memguard.LockedBuffer, error) {
	fd := int(os.Stdin.Fd())

	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		pw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}
		return memguard.NewBufferFromBytes(pw), nil
	}

	// Non-terminal: read a line from stdin
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("reading password from stdin: %w", err)
	}

	// Trim trailing newline
	pw := buf[:n]
	for len(pw) > 0 && (pw[len(pw)-1] == '\n' || pw[len(pw)-1] == '\r') {
		pw = pw[:len(pw)-1]
	}

	return memguard.NewBufferFromBytes(pw), nil
}

// readSecretValue reads a secret value from the terminal or stdin.
func readSecretValue(prompt string) (*memguard.LockedBuffer, error) {
	fd := int(os.Stdin.Fd())

	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		val, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("reading value: %w", err)
		}
		return memguard.NewBufferFromBytes(val), nil
	}

	// Non-terminal: read all of stdin up to limit
	buf := make([]byte, vault.MaxValueSizeExported()+1)
	n, err := os.Stdin.Read(buf)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("reading value from stdin: %w", err)
	}

	val := buf[:n]
	for len(val) > 0 && (val[len(val)-1] == '\n' || val[len(val)-1] == '\r') {
		val = val[:len(val)-1]
	}

	return memguard.NewBufferFromBytes(val), nil
}

// maskValue shows the first 4 and last 4 chars of a value for display.
func maskValue(val string) string {
	if len(val) <= 12 {
		return "****"
	}
	return val[:4] + "..." + val[len(val)-4:]
}

// readFileIfExists returns the contents of a file, or an error if it doesn't exist.
func readFileIfExists(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// writeFileWithDirs writes content to a file, creating parent directories as needed.
func writeFileWithDirs(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
