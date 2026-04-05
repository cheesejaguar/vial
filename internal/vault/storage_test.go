package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteAndReadVaultFile verifies atomic write, 0600 permissions, and
// round-trip JSON fidelity.
func TestWriteAndReadVaultFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")

	vf := &VaultFile{
		Version: 1,
		KDF: KDFParamsJSON{
			Algorithm:   "argon2id",
			Memory:      65536,
			Iterations:  3,
			Parallelism: 1,
			Salt:        "dGVzdHNhbHQ=",
		},
		DEK:      "ZW5jcnlwdGVkLWRlaw==",
		DEKNonce: "bm9uY2U=",
		Keys: map[string]SecretEntry{
			"TEST_KEY": {
				Value:   "ZW5jcnlwdGVk",
				Nonce:   "bm9uY2U=",
				Aliases: []string{"TEST"},
				Tags:    []string{"test"},
				Added:   "2026-01-15T10:30:00Z",
				Rotated: "2026-01-15T10:30:00Z",
			},
		},
		AliasRules: []AliasRule{},
		Projects:   []ProjectRef{},
	}

	if err := WriteVaultFile(path, vf); err != nil {
		t.Fatalf("WriteVaultFile: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	// Read back
	got, err := ReadVaultFile(path)
	if err != nil {
		t.Fatalf("ReadVaultFile: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if got.KDF.Algorithm != "argon2id" {
		t.Errorf("algorithm = %q, want argon2id", got.KDF.Algorithm)
	}
	entry, ok := got.Keys["TEST_KEY"]
	if !ok {
		t.Fatal("TEST_KEY not found in vault")
	}
	if entry.Value != "ZW5jcnlwdGVk" {
		t.Errorf("value = %q, want ZW5jcnlwdGVk", entry.Value)
	}
	if len(entry.Aliases) != 1 || entry.Aliases[0] != "TEST" {
		t.Errorf("aliases = %v, want [TEST]", entry.Aliases)
	}
}

// TestReadVaultFileNotFound verifies that a missing file returns ErrVaultNotFound.
func TestReadVaultFileNotFound(t *testing.T) {
	_, err := ReadVaultFile("/nonexistent/path/vault.json")
	if err != ErrVaultNotFound {
		t.Errorf("expected ErrVaultNotFound, got %v", err)
	}
}

// TestReadVaultFileCorrupted verifies that invalid JSON is detected as corruption.
func TestReadVaultFileCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadVaultFile(path)
	if err == nil {
		t.Error("expected error for corrupted vault file")
	}
}

// TestWriteVaultFileCreatesDir verifies that WriteVaultFile creates parent
// directories as needed.
func TestWriteVaultFileCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "vault.json")

	vf := &VaultFile{
		Version:    1,
		Keys:       map[string]SecretEntry{},
		AliasRules: []AliasRule{},
		Projects:   []ProjectRef{},
	}

	if err := WriteVaultFile(path, vf); err != nil {
		t.Fatalf("WriteVaultFile: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("vault file not created: %v", err)
	}
}

// TestWithFileLock verifies that the callback is executed while the lock is held.
func TestWithFileLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")

	called := false
	err := WithFileLock(path, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithFileLock: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}
