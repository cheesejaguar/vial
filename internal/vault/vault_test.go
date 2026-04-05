package vault

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/awnumar/memguard"
)

// newTestVault creates a VaultManager with fast KDF params for testing.
// It returns the manager already initialized and unlocked.
func newTestVault(t *testing.T) *VaultManager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")
	vm := NewVaultManager(path)
	vm.SetKDFParams(TestKDFParams())

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	if err := vm.Init(password); err != nil {
		t.Fatalf("Init: %v", err)
	}

	return vm
}

// TestVaultInitAndUnlock verifies the full init -> lock -> unlock cycle with
// the same master password.
func TestVaultInitAndUnlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")
	vm := NewVaultManager(path)

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	if err := vm.Init(password); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !vm.IsUnlocked() {
		t.Error("vault should be unlocked after init")
	}

	vm.Lock()
	if vm.IsUnlocked() {
		t.Error("vault should be locked after Lock()")
	}

	// Re-unlock with same password
	password2 := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password2.Destroy()

	if err := vm.Unlock(password2); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	if !vm.IsUnlocked() {
		t.Error("vault should be unlocked after Unlock()")
	}
}

// TestVaultInitPasswordTooShort verifies that Init rejects passwords shorter
// than the minimum length.
func TestVaultInitPasswordTooShort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")
	vm := NewVaultManager(path)

	password := memguard.NewBufferFromBytes([]byte("short"))
	defer password.Destroy()

	err := vm.Init(password)
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}

// TestVaultInitAlreadyExists verifies that Init refuses to overwrite an
// existing vault file.
func TestVaultInitAlreadyExists(t *testing.T) {
	vm := newTestVault(t)

	password := memguard.NewBufferFromBytes([]byte("another-password-long"))
	defer password.Destroy()

	err := vm.Init(password)
	if err != ErrVaultExists {
		t.Errorf("expected ErrVaultExists, got %v", err)
	}
}

// TestVaultUnlockWrongPassword verifies that Unlock returns ErrWrongPassword
// when the master password does not match.
func TestVaultUnlockWrongPassword(t *testing.T) {
	vm := newTestVault(t)
	vm.Lock()

	wrong := memguard.NewBufferFromBytes([]byte("wrong-password-long!"))
	defer wrong.Destroy()

	err := vm.Unlock(wrong)
	if err != ErrWrongPassword {
		t.Errorf("expected ErrWrongPassword, got %v", err)
	}
}

// TestVaultSetGetSecret verifies that a secret can be stored and retrieved
// with its original plaintext value intact.
func TestVaultSetGetSecret(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	value := memguard.NewBufferFromBytes([]byte("sk-proj-abc123"))
	defer value.Destroy()

	if err := vm.SetSecret("OPENAI_API_KEY", value); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	got, err := vm.GetSecret("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	defer got.Destroy()

	if string(got.Bytes()) != "sk-proj-abc123" {
		t.Errorf("got %q, want %q", got.Bytes(), "sk-proj-abc123")
	}
}

// TestVaultGetSecretNotFound verifies that GetSecret returns ErrSecretNotFound
// for a non-existent key.
func TestVaultGetSecretNotFound(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	_, err := vm.GetSecret("NONEXISTENT")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

// TestVaultSetSecretUpdatesExisting verifies that SetSecret overwrites an
// existing key's value while preserving the key name.
func TestVaultSetSecretUpdatesExisting(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	v1 := memguard.NewBufferFromBytes([]byte("value1"))
	defer v1.Destroy()
	if err := vm.SetSecret("KEY", v1); err != nil {
		t.Fatalf("SetSecret 1: %v", err)
	}

	v2 := memguard.NewBufferFromBytes([]byte("value2"))
	defer v2.Destroy()
	if err := vm.SetSecret("KEY", v2); err != nil {
		t.Fatalf("SetSecret 2: %v", err)
	}

	got, err := vm.GetSecret("KEY")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	defer got.Destroy()

	if string(got.Bytes()) != "value2" {
		t.Errorf("got %q, want %q", got.Bytes(), "value2")
	}
}

// TestVaultListSecrets verifies that ListSecrets returns all stored key names.
func TestVaultListSecrets(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	keys := []string{"KEY_A", "KEY_B", "KEY_C"}
	for _, k := range keys {
		v := memguard.NewBufferFromBytes([]byte("val-" + k))
		if err := vm.SetSecret(k, v); err != nil {
			t.Fatalf("SetSecret %s: %v", k, err)
		}
		v.Destroy()
	}

	list := vm.ListSecrets()
	if len(list) != 3 {
		t.Fatalf("ListSecrets returned %d, want 3", len(list))
	}

	found := map[string]bool{}
	for _, info := range list {
		found[info.Key] = true
	}
	for _, k := range keys {
		if !found[k] {
			t.Errorf("key %s not found in list", k)
		}
	}
}

// TestVaultRemoveSecret verifies that a removed key is no longer retrievable.
func TestVaultRemoveSecret(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	v := memguard.NewBufferFromBytes([]byte("value"))
	defer v.Destroy()
	if err := vm.SetSecret("KEY", v); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	if err := vm.RemoveSecret("KEY"); err != nil {
		t.Fatalf("RemoveSecret: %v", err)
	}

	_, err := vm.GetSecret("KEY")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound after remove, got %v", err)
	}
}

// TestVaultRemoveSecretNotFound verifies that removing a non-existent key
// returns ErrSecretNotFound.
func TestVaultRemoveSecretNotFound(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	err := vm.RemoveSecret("NONEXISTENT")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

// TestVaultOperationsWhenLocked verifies that Set, Get, and Remove all return
// ErrVaultLocked when the vault is locked.
func TestVaultOperationsWhenLocked(t *testing.T) {
	vm := newTestVault(t)
	vm.Lock()

	v := memguard.NewBufferFromBytes([]byte("value"))
	defer v.Destroy()

	if err := vm.SetSecret("KEY", v); err != ErrVaultLocked {
		t.Errorf("SetSecret when locked: expected ErrVaultLocked, got %v", err)
	}

	if _, err := vm.GetSecret("KEY"); err != ErrVaultLocked {
		t.Errorf("GetSecret when locked: expected ErrVaultLocked, got %v", err)
	}

	if err := vm.RemoveSecret("KEY"); err != ErrVaultLocked {
		t.Errorf("RemoveSecret when locked: expected ErrVaultLocked, got %v", err)
	}
}

// TestVaultGetSetMetadata verifies that metadata (aliases, provider, tags) can
// be stored and retrieved for a secret.
func TestVaultGetSetMetadata(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	v := memguard.NewBufferFromBytes([]byte("value"))
	defer v.Destroy()
	if err := vm.SetSecret("KEY", v); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	meta := SecretMetadata{
		Aliases:  []string{"MY_KEY"},
		Provider: "openai",
		Tags:     []string{"ai", "prod"},
	}
	if err := vm.SetMetadata("KEY", meta); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	got, err := vm.GetMetadata("KEY")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}

	if len(got.Aliases) != 1 || got.Aliases[0] != "MY_KEY" {
		t.Errorf("aliases = %v, want [MY_KEY]", got.Aliases)
	}
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want openai", got.Provider)
	}
}

// TestUnlockWithDEK_GarbageBytesRejected verifies that UnlockWithDEK rejects
// invalid DEK bytes and leaves the vault locked.
func TestUnlockWithDEK_GarbageBytesRejected(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	// Store a secret so the vault has encrypted data
	secret := memguard.NewBufferFromBytes([]byte("super-secret-value"))
	defer secret.Destroy()
	if err := vm.SetSecret("TEST_KEY", secret); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	// Lock the vault, then try to unlock with garbage DEK bytes
	vm.Lock()

	garbageDEK := []byte("this-is-32-bytes-of-garbage!!!!!")
	err := vm.UnlockWithDEK(garbageDEK)
	if err != ErrInvalidDEK {
		t.Errorf("expected ErrInvalidDEK, got %v", err)
	}
	if vm.IsUnlocked() {
		t.Error("vault should remain locked after failed UnlockWithDEK")
	}
}

// TestValidateKeyName exercises valid and invalid POSIX environment variable
// names against the key name validator.
func TestValidateKeyName(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		// Valid names
		{"simple uppercase", "API_KEY", false},
		{"simple lowercase", "api_key", false},
		{"mixed case", "MyKey_01", false},
		{"single letter", "X", false},
		{"underscore prefix", "_PRIVATE", false},
		{"just underscore", "_", false},
		{"max length", strings.Repeat("A", 256), false},

		// Invalid names
		{"empty string", "", true},
		{"starts with digit", "1KEY", true},
		{"contains space", "MY KEY", true},
		{"contains dash", "MY-KEY", true},
		{"contains dot", "my.key", true},
		{"contains equals", "KEY=VAL", true},
		{"contains slash", "path/key", true},
		{"too long", strings.Repeat("A", 257), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeyName(tt.key)
			if tt.wantErr && err != ErrInvalidKeyName {
				t.Errorf("ValidateKeyName(%q) = %v, want ErrInvalidKeyName", tt.key, err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateKeyName(%q) = %v, want nil", tt.key, err)
			}
		})
	}
}

// TestChangePassword verifies the full password rotation flow: change password,
// unlock with new password, verify secrets survive, and confirm old password
// is rejected.
func TestChangePassword(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	// Store a secret before changing password
	secret := memguard.NewBufferFromBytes([]byte("my-secret-value"))
	defer secret.Destroy()
	if err := vm.SetSecret("TEST_KEY", secret); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	oldPw := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer oldPw.Destroy()
	newPw := memguard.NewBufferFromBytes([]byte("new-password-12chars!"))
	defer newPw.Destroy()

	if err := vm.ChangePassword(oldPw, newPw); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	// Lock and unlock with new password should succeed
	vm.Lock()

	newPw2 := memguard.NewBufferFromBytes([]byte("new-password-12chars!"))
	defer newPw2.Destroy()
	if err := vm.Unlock(newPw2); err != nil {
		t.Fatalf("Unlock with new password: %v", err)
	}

	// Verify the stored secret is still accessible
	got, err := vm.GetSecret("TEST_KEY")
	if err != nil {
		t.Fatalf("GetSecret after password change: %v", err)
	}
	defer got.Destroy()
	if string(got.Bytes()) != "my-secret-value" {
		t.Errorf("got %q, want %q", got.Bytes(), "my-secret-value")
	}

	// Lock and try old password — should fail
	vm.Lock()

	oldPw2 := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer oldPw2.Destroy()
	err = vm.Unlock(oldPw2)
	if err != ErrWrongPassword {
		t.Errorf("Unlock with old password: expected ErrWrongPassword, got %v", err)
	}
}

// TestSetSecretRejectsInvalidKeyName verifies that SetSecret rejects keys that
// do not match POSIX env var naming rules.
func TestSetSecretRejectsInvalidKeyName(t *testing.T) {
	vm := newTestVault(t)
	defer vm.Lock()

	val := memguard.NewBufferFromBytes([]byte("value"))
	defer val.Destroy()

	invalidKeys := []string{"", "1BAD", "has space", "has-dash"}
	for _, key := range invalidKeys {
		if err := vm.SetSecret(key, val); err != ErrInvalidKeyName {
			t.Errorf("SetSecret(%q) = %v, want ErrInvalidKeyName", key, err)
		}
	}
}

// TestVaultFullLifecycle exercises init, set, list, lock, unlock, get, and
// remove in sequence to validate the complete vault workflow.
func TestVaultFullLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.json")
	vm := NewVaultManager(path)
	vm.SetKDFParams(TestKDFParams())

	// Init
	pw := memguard.NewBufferFromBytes([]byte("lifecycle-test-pass"))
	defer pw.Destroy()
	if err := vm.Init(pw); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Set secrets
	for _, kv := range []struct{ k, v string }{
		{"OPENAI_API_KEY", "sk-123"},
		{"STRIPE_KEY", "sk_test_456"},
		{"DB_URL", "postgres://localhost/db"},
	} {
		val := memguard.NewBufferFromBytes([]byte(kv.v))
		if err := vm.SetSecret(kv.k, val); err != nil {
			t.Fatalf("SetSecret %s: %v", kv.k, err)
		}
		val.Destroy()
	}

	// Verify list
	list := vm.ListSecrets()
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3", len(list))
	}

	// Lock and re-unlock
	vm.Lock()
	pw2 := memguard.NewBufferFromBytes([]byte("lifecycle-test-pass"))
	defer pw2.Destroy()
	if err := vm.Unlock(pw2); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	// Verify secrets survive lock/unlock
	got, err := vm.GetSecret("STRIPE_KEY")
	if err != nil {
		t.Fatalf("GetSecret after unlock: %v", err)
	}
	if string(got.Bytes()) != "sk_test_456" {
		t.Errorf("got %q, want sk_test_456", got.Bytes())
	}
	got.Destroy()

	// Remove a secret
	if err := vm.RemoveSecret("DB_URL"); err != nil {
		t.Fatalf("RemoveSecret: %v", err)
	}

	list = vm.ListSecrets()
	if len(list) != 2 {
		t.Errorf("list len after remove = %d, want 2", len(list))
	}

	vm.Lock()
}
