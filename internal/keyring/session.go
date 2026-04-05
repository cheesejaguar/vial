// Package keyring manages the short-lived session cache that keeps the vault
// unlocked across invocations without requiring the master password every time.
//
// When the vault is first unlocked, the plaintext DEK (data-encryption key) is
// stored in the OS keyring (macOS Keychain, SecretService on Linux, Windows
// Credential Manager) together with an expiry timestamp. Subsequent commands
// retrieve the DEK from the keyring and skip the Argon2id derivation, making
// them near-instant.
//
// Security model: the DEK is stored as a base64-encoded string inside the OS
// keyring entry. The entry key is derived from the SHA-256 hash of the vault
// file's absolute path, so different vault files each have their own isolated
// session. The expiry is enforced client-side on retrieval; a stale entry is
// deleted immediately.
package keyring

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// ErrSessionNotFound is returned when no keyring entry exists for the vault.
var ErrSessionNotFound = errors.New("no active session found")

// ErrSessionExpired is returned when a keyring entry exists but its TTL has
// elapsed. The stale entry is deleted before this error is returned.
var ErrSessionExpired = errors.New("session has expired")

// serviceName is the keyring service identifier used for all vial entries.
const serviceName = "vial"

// SessionManager handles DEK caching in the OS keychain.
// The zero value is ready to use; create one with NewSessionManager.
type SessionManager struct{}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Store caches the DEK bytes in the OS keyring with an expiry timestamp.
//
// The stored payload format is "unix_epoch:base64_dek", where unix_epoch is
// the Unix timestamp (seconds) at which the session expires and base64_dek is
// the standard-encoding base64 representation of the raw DEK bytes.
//
// The caller should zero dekBytes after this call returns; this package does
// not retain a reference to the slice.
func (s *SessionManager) Store(vaultPath string, dekBytes []byte, ttl time.Duration) error {
	user := vaultID(vaultPath)
	expiry := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d:%s", expiry, base64.StdEncoding.EncodeToString(dekBytes))

	if err := keyring.Set(serviceName, user, payload); err != nil {
		return fmt.Errorf("storing session: %w", err)
	}
	return nil
}

// Retrieve returns the cached DEK if the session has not expired.
//
// If the entry is missing, malformed, or past its expiry timestamp, the stale
// entry is removed and a sentinel error (ErrSessionNotFound or
// ErrSessionExpired) is returned so the caller can fall back to an interactive
// password prompt.
func (s *SessionManager) Retrieve(vaultPath string) ([]byte, error) {
	user := vaultID(vaultPath)

	payload, err := keyring.Get(serviceName, user)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("retrieving session: %w", err)
	}

	// Payload format: "expiry_unix:base64_dek"
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		// Entry is corrupt; remove it so the user is prompted on the next call.
		s.Clear(vaultPath)
		return nil, ErrSessionNotFound
	}

	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.Clear(vaultPath)
		return nil, ErrSessionNotFound
	}

	// Enforce TTL client-side. The OS keyring does not natively support
	// entry expiry, so we check the embedded timestamp here.
	if time.Now().Unix() > expiry {
		s.Clear(vaultPath)
		return nil, ErrSessionExpired
	}

	dekBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		s.Clear(vaultPath)
		return nil, ErrSessionNotFound
	}

	return dekBytes, nil
}

// Clear removes the cached session for the given vault path.
//
// It is safe to call Clear when no session exists; the underlying keyring
// ErrNotFound is silently ignored.
func (s *SessionManager) Clear(vaultPath string) error {
	user := vaultID(vaultPath)
	err := keyring.Delete(serviceName, user)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("clearing session: %w", err)
	}
	return nil
}

// vaultID creates a stable, short identifier for a vault path used as the
// keyring account name.
//
// It resolves the path to an absolute form before hashing so that
// "./vault.json" and "/home/user/.local/share/vial/vault.json" produce the
// same identifier. The first 8 bytes of the SHA-256 hash are encoded as a
// lowercase hex string (16 characters), which is short enough to be a
// comfortable keyring account field while still having negligible collision
// probability across a single user's set of vault files.
func vaultID(vaultPath string) string {
	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		// Fall back to the raw path if resolution fails (e.g. in unit tests
		// running under a restricted filesystem).
		absPath = vaultPath
	}
	hash := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("%x", hash[:8])
}
