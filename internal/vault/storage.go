package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const vaultFilePerms = 0600

// ReadVaultFile reads and parses the vault file from disk.
// The caller must hold or acquire a lock before calling this in a write context.
func ReadVaultFile(path string) (*VaultFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrVaultNotFound
		}
		return nil, fmt.Errorf("reading vault file: %w", err)
	}

	var vf VaultFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVaultCorrupted, err)
	}

	return &vf, nil
}

// WriteVaultFile atomically writes the vault file to disk.
// It writes to a temp file first then renames, preventing corruption on crash.
func WriteVaultFile(path string, vf *VaultFile) error {
	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, vaultFilePerms); err != nil {
		return fmt.Errorf("writing temp vault file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("renaming vault file: %w", err)
	}

	return nil
}

// WithFileLock acquires an exclusive file lock on the vault file for the
// duration of fn. This prevents concurrent read-modify-write corruption.
func WithFileLock(path string, fn func() error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring file lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return fn()
}
