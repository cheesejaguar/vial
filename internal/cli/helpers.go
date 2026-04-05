// Package helpers provides shared state and utilities used across all CLI
// sub-commands: config loading, vault access, password/value input, and small
// file-system conveniences.
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

// Package-level shared state. All CLI commands share a single logger, config
// pointer, and session manager so they behave consistently and avoid redundant
// keyring lookups within a single invocation.
var (
	// logger writes structured log output to stderr so it never pollutes
	// stdout, which some callers may pipe or redirect for machine consumption.
	logger = log.NewWithOptions(os.Stderr, log.Options{})

	// cfg is populated by loadConfig() on the first command that needs it.
	// Commands must call loadConfig() before accessing any cfg field.
	cfg *config.Config

	// session manages the OS keyring cache of the encrypted DEK. Caching
	// the raw DEK bytes (not the master password) means the user is only
	// prompted once per session_timeout window rather than on every command.
	session = keyring.NewSessionManager()
)

// loadConfig loads the application configuration from the YAML file located at
// cfgFile (if set via --config) or from the default path
// ~/.config/vial/config.yaml. The populated config is stored in the package-
// level cfg variable so subsequent helpers can access it without re-reading.
func loadConfig() error {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return nil
}

// openVault creates a VaultManager pointed at cfg.VaultPath and attempts a
// zero-interaction unlock using the OS keyring session cache. If a valid cached
// DEK exists the returned vault is already unlocked; otherwise it is returned
// in a locked state for the caller to unlock via a password prompt or env var.
//
// The session cache holds the raw DEK bytes (not the master password), so an
// attacker who steals the keyring entry can decrypt the vault file directly.
// This is an intentional trade-off: the DEK is also protected by the vault
// file's filesystem permissions (0600) and the keyring's OS-level access
// control.
func openVault() (*vault.VaultManager, error) {
	if err := loadConfig(); err != nil {
		return nil, err
	}

	vm := vault.NewVaultManager(cfg.VaultPath)

	// Try session cache first — avoids a password prompt on every command
	// during the session_timeout window (default 4 h).
	dekBytes, err := session.Retrieve(cfg.VaultPath)
	if err == nil {
		if err := vm.UnlockWithDEK(dekBytes); err == nil {
			return vm, nil
		}
		// The cached DEK no longer matches the vault (e.g. after a rekey).
		// Remove the stale entry so we fall through to the password prompt.
		session.Clear(cfg.VaultPath)
	}

	return vm, nil
}

// requireUnlockedVault returns a fully unlocked VaultManager, trying three
// unlock strategies in priority order:
//
//  1. OS keyring session cache — checked by openVault(); no user interaction.
//  2. VIAL_MASTER_KEY environment variable — intended for CI/CD pipelines and
//     Docker containers where interactive input is unavailable.
//  3. Interactive terminal prompt — used when running in a real shell session;
//     the DEK is cached in the keyring after a successful unlock so that
//     subsequent commands within the session_timeout window skip this step.
//
// If the vault file does not exist at any point, a helpful "run vial init"
// message is returned rather than the raw filesystem error.
func requireUnlockedVault() (*vault.VaultManager, error) {
	vm, err := openVault()
	if err != nil {
		if errors.Is(err, vault.ErrVaultNotFound) {
			return nil, fmt.Errorf("no vault found — run 'vial init' to create one")
		}
		return nil, err
	}

	// Strategy 1 succeeded inside openVault().
	if vm.IsUnlocked() {
		return vm, nil
	}

	// Strategy 2: headless/CI mode via environment variable.
	if masterKey := os.Getenv("VIAL_MASTER_KEY"); masterKey != "" {
		// Wrap in a LockedBuffer so the key material is mlock'd and zeroed
		// when Destroy is called at the end of this scope.
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

	// Strategy 3: interactive terminal prompt.
	// Guard with isInteractive() so we never hang waiting for input in a
	// pipeline or cron job where no human is present.
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

	// Seed the keyring cache so the next command in the same session skips
	// the password prompt. A keyring error is non-fatal: the vault is already
	// unlocked and the user can re-enter the password next time.
	if dekBytes := vm.DEKBytes(); dekBytes != nil {
		if err := session.Store(cfg.VaultPath, dekBytes, cfg.SessionTimeout); err != nil {
			logger.Warn("Could not cache session in keyring", "err", err)
		}
	}

	return vm, nil
}

// isInteractive reports whether stdin is connected to a real terminal. Commands
// that display interactive UI (huh forms, password prompts) must check this
// before attempting to read from stdin to avoid hanging in piped or scripted
// invocations.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// readPassword reads a password from the terminal with echo disabled, or from
// stdin when input is piped (e.g. in tests or CI scripts). The prompt is always
// written to stderr so it does not pollute a captured stdout stream. A trailing
// newline is printed after the hidden input to restore the terminal cursor to a
// new line.
//
// The caller is responsible for calling Destroy() on the returned LockedBuffer
// once it is no longer needed.
func readPassword(prompt string) (*memguard.LockedBuffer, error) {
	fd := int(os.Stdin.Fd())

	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		pw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr) // restore cursor position after hidden input
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}
		return memguard.NewBufferFromBytes(pw), nil
	}

	// Non-terminal: read a single line from stdin (e.g. echo "pass" | vial …).
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("reading password from stdin: %w", err)
	}

	// Strip the trailing newline that most shells append to piped strings.
	pw := buf[:n]
	for len(pw) > 0 && (pw[len(pw)-1] == '\n' || pw[len(pw)-1] == '\r') {
		pw = pw[:len(pw)-1]
	}

	return memguard.NewBufferFromBytes(pw), nil
}

// readSecretValue reads a secret value from the terminal (with echo disabled)
// or from stdin when input is piped. Unlike readPassword, it enforces the
// vault's MaxValueSize limit to reject values that would be rejected at storage
// time anyway, giving a clearer error earlier in the flow.
//
// Secret values are intentionally never accepted as positional CLI arguments
// to avoid them appearing in shell history or process listings.
//
// The caller is responsible for calling Destroy() on the returned LockedBuffer.
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

	// Allocate one byte beyond the limit so we can detect oversized input
	// and return a meaningful error rather than silently truncating.
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

// maskValue returns a partially redacted representation of val suitable for
// display in terminal output. Values of 12 characters or fewer are replaced
// entirely with "****" to avoid leaking meaningful prefix/suffix information
// about short tokens or passwords.
func maskValue(val string) string {
	if len(val) <= 12 {
		return "****"
	}
	return val[:4] + "..." + val[len(val)-4:]
}

// readFileIfExists returns the raw byte contents of the file at path. It is a
// thin wrapper around os.ReadFile provided for semantic clarity at call sites
// where the intent is "read only if present, let the caller handle absence."
func readFileIfExists(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// writeFileWithDirs writes data to path, creating any missing parent directories
// with mode 0700 before writing. This ensures that vial-managed directories
// (e.g. ~/.config/vial/) are created with restrictive permissions even on
// systems where the umask would allow group/other read access.
func writeFileWithDirs(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
