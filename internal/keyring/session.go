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

var (
	ErrSessionNotFound = errors.New("no active session found")
	ErrSessionExpired  = errors.New("session has expired")
)

const serviceName = "vial"

// SessionManager handles DEK caching in the OS keychain.
type SessionManager struct{}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// Store caches the DEK bytes in the OS keyring with an expiry timestamp.
// The stored payload format is: "unix_epoch:base64_dek"
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
func (s *SessionManager) Retrieve(vaultPath string) ([]byte, error) {
	user := vaultID(vaultPath)

	payload, err := keyring.Get(serviceName, user)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("retrieving session: %w", err)
	}

	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		s.Clear(vaultPath)
		return nil, ErrSessionNotFound
	}

	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.Clear(vaultPath)
		return nil, ErrSessionNotFound
	}

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

// Clear removes the cached session.
func (s *SessionManager) Clear(vaultPath string) error {
	user := vaultID(vaultPath)
	err := keyring.Delete(serviceName, user)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("clearing session: %w", err)
	}
	return nil
}

// vaultID creates a stable, short identifier for a vault path.
func vaultID(vaultPath string) string {
	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		absPath = vaultPath
	}
	hash := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("%x", hash[:8])
}
