package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// vaultFilePerms restricts the vault file to owner-only read/write (0600).
// This prevents other users on the system from reading encrypted secrets.
const vaultFilePerms = 0600

// ReadVaultFile reads and parses the JSON vault file at the given path.
// In a read-modify-write context the caller must hold a file lock (see
// WithFileLock) before calling this function. For read-only operations such
// as GetSecret or ListSecrets, no lock is required because the vault file is
// always written atomically via rename.
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

// WriteVaultFile atomically writes the vault file to disk with 0600 permissions.
// It serializes to a .tmp file first, then renames over the target path. This
// ensures that a crash or power loss during the write never leaves a truncated
// or partially-written vault file. The parent directory is created with 0700
// permissions if it does not exist.
func WriteVaultFile(path string, vf *VaultFile) error {
	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	// Write to a temporary file alongside the vault, then atomically rename.
	// On POSIX systems, rename is atomic within the same filesystem.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, vaultFilePerms); err != nil {
		return fmt.Errorf("writing temp vault file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup on rename failure
		return fmt.Errorf("renaming vault file: %w", err)
	}

	return nil
}

// WithFileLock acquires an exclusive advisory lock (syscall.Flock LOCK_EX) on
// a .lock sidecar file for the duration of fn, then releases it. This
// serializes concurrent read-modify-write operations on the vault file from
// multiple processes (e.g., parallel CLI invocations). The lock file is created
// in the same directory as the vault with 0600 permissions.
func WithFileLock(path string, fn func() error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	// Use a separate .lock file rather than locking the vault file itself,
	// because the vault file is atomically replaced via rename on each write.
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
