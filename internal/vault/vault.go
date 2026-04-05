// Package vault implements an encrypted secret store using AES-256-GCM with
// Argon2id key derivation.
//
// The encryption model follows a two-tier key hierarchy:
//
//	Master Password -> Argon2id (64 MiB, 3 iter) -> KEK -> encrypts DEK
//	DEK encrypts each secret value individually via AES-256-GCM
//
// The vault file uses a SOPS-style format where key names are plaintext JSON
// map keys and values are individually encrypted with per-value random nonces.
// This allows readable diffs and selective access while keeping values secret.
//
// All key material is held in memguard LockedBuffers (mlock'd, guard-paged
// memory). The caller who receives a *memguard.LockedBuffer owns it and must
// call Destroy() when done. The DEK is owned by VaultManager and destroyed
// when Lock() is called.
//
// Vault file writes are atomic (temp file + os.Rename) and protected by
// syscall.Flock to prevent concurrent read-modify-write corruption.
package vault

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"time"

	"github.com/awnumar/memguard"
)

// validKeyNameRe enforces POSIX environment variable naming: letters, digits,
// and underscores, starting with a letter or underscore.
var validKeyNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// maxKeyNameLength caps key names to prevent abuse in the JSON map.
const maxKeyNameLength = 256

// ValidateKeyName checks that name is a valid POSIX environment variable name
// (letters, digits, underscore; must start with letter or underscore) and is
// at most 256 characters long. It is exported for use in CLI code.
func ValidateKeyName(name string) error {
	if len(name) == 0 || len(name) > maxKeyNameLength || !validKeyNameRe.MatchString(name) {
		return ErrInvalidKeyName
	}
	return nil
}

// minPasswordLength is the minimum acceptable master password length.
// Shorter passwords are rejected at Init and ChangePassword boundaries.
const minPasswordLength = 12

// VaultManager implements the Vault interface, managing the lifecycle of the
// encrypted vault file and the in-memory DEK. It is the single owner of the
// DEK buffer: callers must not retain references to it, and Lock() destroys it.
type VaultManager struct {
	path        string                 // absolute path to the vault JSON file
	dek         *memguard.LockedBuffer // decrypted DEK; nil when locked
	params      KDFParams              // KDF params read from the vault file on last unlock
	kdfOverride *KDFParams             // overrides DefaultKDFParams when set (test-only)
}

// NewVaultManager creates a new VaultManager for the vault at the given path.
func NewVaultManager(path string) *VaultManager {
	return &VaultManager{path: path}
}

// SetKDFParams overrides the default KDF parameters (use only for testing).
func (v *VaultManager) SetKDFParams(params KDFParams) {
	v.kdfOverride = &params
}

// Path returns the filesystem path to the vault file.
func (v *VaultManager) Path() string { return v.path }

// IsUnlocked reports whether the DEK is currently held in memory.
func (v *VaultManager) IsUnlocked() bool { return v.dek != nil }

// Version returns the vault file format version.
func (v *VaultManager) Version() int { return 1 }

// Init creates a new vault file encrypted with the given master password.
// It generates a random salt, derives a KEK via Argon2id, generates a random
// DEK, encrypts the DEK under the KEK, and writes the initial vault file.
// On success the vault is left unlocked (DEK cached in memory) so the caller
// can immediately store secrets. The password buffer is not consumed; the
// caller retains ownership.
func (v *VaultManager) Init(password *memguard.LockedBuffer) error {
	if password.Size() < minPasswordLength {
		return ErrPasswordTooShort
	}

	// Refuse to overwrite an existing vault to prevent accidental data loss.
	if _, err := ReadVaultFile(v.path); err == nil {
		return ErrVaultExists
	}

	// Fresh random salt for the Argon2id KDF.
	salt, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	params := DefaultKDFParams()
	if v.kdfOverride != nil {
		params = *v.kdfOverride
	}
	params.Salt = salt

	// Derive KEK from password. The KEK is ephemeral and destroyed after
	// encrypting the DEK -- it is never stored or cached.
	kek, err := DeriveKEK(password, params)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	defer kek.Destroy()

	// Generate the DEK that will encrypt individual secret values.
	dek, err := GenerateDEK()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// Encrypt the DEK with the KEK. A fresh random nonce is generated inside
	// EncryptAESGCM for every call.
	encDEK, dekNonce, err := EncryptAESGCM(kek, dek.Bytes())
	if err != nil {
		dek.Destroy()
		return fmt.Errorf("init: encrypting DEK: %w", err)
	}

	// Build the initial vault file with empty secret, alias, and project maps.
	vf := &VaultFile{
		Version: 1,
		KDF: KDFParamsJSON{
			Algorithm:   params.Algorithm,
			Memory:      params.Memory,
			Iterations:  params.Iterations,
			Parallelism: params.Parallelism,
			Salt:        base64.StdEncoding.EncodeToString(salt),
		},
		DEK:        base64.StdEncoding.EncodeToString(encDEK),
		DEKNonce:   base64.StdEncoding.EncodeToString(dekNonce),
		Keys:       map[string]SecretEntry{},
		AliasRules: []AliasRule{},
		Projects:   []ProjectRef{},
	}

	if err := WriteVaultFile(v.path, vf); err != nil {
		dek.Destroy()
		return fmt.Errorf("init: %w", err)
	}

	// Keep vault unlocked after init so the caller can store secrets immediately.
	// VaultManager now owns this DEK buffer and will destroy it in Lock().
	v.dek = dek
	v.params = params
	return nil
}

// Unlock derives the KEK from the password, decrypts the DEK, and caches it
// in mlock'd memory. A failed GCM authentication (wrong password) is surfaced
// as ErrWrongPassword rather than leaking the underlying crypto error. After
// Unlock returns nil, the vault is ready for secret operations.
func (v *VaultManager) Unlock(password *memguard.LockedBuffer) error {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(vf.KDF.Salt)
	if err != nil {
		return fmt.Errorf("unlock: decoding salt: %w", err)
	}

	// Reconstruct KDF params from what was stored in the vault file so that
	// the same Argon2id cost parameters and salt produce the same KEK.
	params := KDFParams{
		Algorithm:   vf.KDF.Algorithm,
		Memory:      vf.KDF.Memory,
		Iterations:  vf.KDF.Iterations,
		Parallelism: vf.KDF.Parallelism,
		Salt:        salt,
	}

	kek, err := DeriveKEK(password, params)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	defer kek.Destroy() // KEK is ephemeral; only the DEK is cached.

	encDEK, err := base64.StdEncoding.DecodeString(vf.DEK)
	if err != nil {
		return fmt.Errorf("unlock: decoding DEK: %w", err)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(vf.DEKNonce)
	if err != nil {
		return fmt.Errorf("unlock: decoding DEK nonce: %w", err)
	}

	// GCM decryption authenticates the ciphertext. Failure here means the
	// password was wrong (KEK mismatch), so we return a user-friendly error.
	dekBytes, err := DecryptAESGCM(kek, encDEK, dekNonce)
	if err != nil {
		return ErrWrongPassword
	}

	// VaultManager takes ownership of the DEK buffer. It will be destroyed in Lock().
	v.dek = memguard.NewBufferFromBytes(dekBytes)
	v.params = params
	return nil
}

// Lock zeroes and destroys the in-memory DEK, releasing the mlock'd memory.
// After Lock returns, all secret operations will return ErrVaultLocked.
// Calling Lock on an already-locked vault is safe and has no effect.
func (v *VaultManager) Lock() {
	if v.dek != nil {
		v.dek.Destroy()
		v.dek = nil
	}
}

// SetSecret encrypts and stores a secret value in the vault under the given
// key name. The value is encrypted with the cached DEK using AES-256-GCM with
// a fresh random nonce. The entire read-modify-write is performed under a file
// lock to prevent concurrent corruption. When updating an existing key, user
// metadata (aliases, provider, tags, rotation policy, and original add time)
// is preserved; only the encrypted value and the "rotated" timestamp change.
// The caller retains ownership of the value buffer.
func (v *VaultManager) SetSecret(key string, value *memguard.LockedBuffer) error {
	if err := ValidateKeyName(key); err != nil {
		return err
	}
	if !v.IsUnlocked() {
		return ErrVaultLocked
	}
	if value.Size() > maxValueSize {
		return ErrValueTooLarge
	}

	var retErr error
	err := WithFileLock(v.path, func() error {
		vf, err := ReadVaultFile(v.path)
		if err != nil {
			return err
		}

		// Every encryption generates a fresh 12-byte nonce from crypto/rand,
		// even when overwriting the same key. Never reuse nonces with AES-GCM.
		ciphertext, nonce, err := EncryptAESGCM(v.dek, value.Bytes())
		if err != nil {
			return fmt.Errorf("encrypting secret: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339)

		existing, exists := vf.Keys[key]
		entry := SecretEntry{
			Value:   base64.StdEncoding.EncodeToString(ciphertext),
			Nonce:   base64.StdEncoding.EncodeToString(nonce),
			Aliases: []string{},
			Tags:    []string{},
			Added:   now,
			Rotated: now,
		}

		// Preserve user-defined metadata when rotating an existing secret so
		// that aliases, tags, and rotation policy survive value updates.
		if exists {
			entry.Aliases = existing.Aliases
			entry.Provider = existing.Provider
			entry.Tags = existing.Tags
			entry.Added = existing.Added               // keep original add time
			entry.RotationDays = existing.RotationDays // keep rotation policy
		}

		vf.Keys[key] = entry
		return WriteVaultFile(v.path, vf)
	})
	if err != nil {
		retErr = fmt.Errorf("set secret %q: %w", key, err)
	}
	return retErr
}

// GetSecret decrypts and returns a secret value in a memguard LockedBuffer.
// The caller owns the returned buffer and must call Destroy() on it when done.
// This is a read-only operation and does not acquire a file lock.
func (v *VaultManager) GetSecret(key string) (*memguard.LockedBuffer, error) {
	if !v.IsUnlocked() {
		return nil, ErrVaultLocked
	}

	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}

	entry, ok := vf.Keys[key]
	if !ok {
		return nil, ErrSecretNotFound
	}

	ciphertext, err := base64.StdEncoding.DecodeString(entry.Value)
	if err != nil {
		return nil, fmt.Errorf("get secret: decoding value: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(entry.Nonce)
	if err != nil {
		return nil, fmt.Errorf("get secret: decoding nonce: %w", err)
	}

	plaintext, err := DecryptAESGCM(v.dek, ciphertext, nonce)
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}

	return memguard.NewBufferFromBytes(plaintext), nil
}

// ListSecrets returns all key names with metadata (no decrypted values).
// It does not require the vault to be unlocked because key names and metadata
// are stored in plaintext. Returns nil if the vault file cannot be read.
func (v *VaultManager) ListSecrets() []SecretInfo {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return nil
	}

	infos := make([]SecretInfo, 0, len(vf.Keys))
	for key, entry := range vf.Keys {
		added, _ := time.Parse(time.RFC3339, entry.Added)
		rotated, _ := time.Parse(time.RFC3339, entry.Rotated)
		infos = append(infos, SecretInfo{
			Key: key,
			Metadata: SecretMetadata{
				Aliases:      entry.Aliases,
				Provider:     entry.Provider,
				Tags:         entry.Tags,
				Added:        added,
				Rotated:      rotated,
				RotationDays: entry.RotationDays,
			},
		})
	}
	return infos
}

// RemoveSecret removes a key and its encrypted value from the vault file.
// The operation is performed under a file lock for atomicity.
func (v *VaultManager) RemoveSecret(key string) error {
	if !v.IsUnlocked() {
		return ErrVaultLocked
	}

	return WithFileLock(v.path, func() error {
		vf, err := ReadVaultFile(v.path)
		if err != nil {
			return fmt.Errorf("remove secret: %w", err)
		}

		if _, ok := vf.Keys[key]; !ok {
			return ErrSecretNotFound
		}

		delete(vf.Keys, key)
		return WriteVaultFile(v.path, vf)
	})
}

// GetMetadata returns the plaintext metadata (aliases, provider, tags,
// timestamps, rotation policy) for a stored secret without decrypting its value.
func (v *VaultManager) GetMetadata(key string) (*SecretMetadata, error) {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return nil, fmt.Errorf("get metadata: %w", err)
	}

	entry, ok := vf.Keys[key]
	if !ok {
		return nil, ErrSecretNotFound
	}

	added, _ := time.Parse(time.RFC3339, entry.Added)
	rotated, _ := time.Parse(time.RFC3339, entry.Rotated)

	return &SecretMetadata{
		Aliases:      entry.Aliases,
		Provider:     entry.Provider,
		Tags:         entry.Tags,
		Added:        added,
		Rotated:      rotated,
		RotationDays: entry.RotationDays,
	}, nil
}

// SetMetadata updates the plaintext metadata for a stored secret without
// touching the encrypted value. The operation is performed under a file lock.
func (v *VaultManager) SetMetadata(key string, meta SecretMetadata) error {
	if !v.IsUnlocked() {
		return ErrVaultLocked
	}

	return WithFileLock(v.path, func() error {
		vf, err := ReadVaultFile(v.path)
		if err != nil {
			return err
		}

		entry, ok := vf.Keys[key]
		if !ok {
			return ErrSecretNotFound
		}

		entry.Aliases = meta.Aliases
		entry.Provider = meta.Provider
		entry.Tags = meta.Tags
		entry.RotationDays = meta.RotationDays
		vf.Keys[key] = entry
		return WriteVaultFile(v.path, vf)
	})
}

// DEKBytes returns the raw DEK bytes for session caching (e.g., in the OS
// keyring). The returned slice points into the memguard buffer; the caller
// must not modify, free, or retain it beyond the VaultManager's lifetime.
// Returns nil when the vault is locked.
func (v *VaultManager) DEKBytes() []byte {
	if v.dek == nil {
		return nil
	}
	return v.dek.Bytes()
}

// UnlockWithDEK sets the DEK directly from raw bytes, bypassing password-based
// key derivation. This is used when restoring a session from the OS keyring
// cache. If the vault contains any secrets, the DEK is validated by attempting
// a trial decryption of one entry. This guards against stale or corrupt cached
// keys. On success, VaultManager takes ownership of a copy of dekBytes in a
// new LockedBuffer.
func (v *VaultManager) UnlockWithDEK(dekBytes []byte) error {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return err
	}

	// Verify the cached DEK is still valid by trial-decrypting one entry.
	// An empty vault (no keys) is accepted without verification.
	for _, entry := range vf.Keys {
		ciphertext, err := base64.StdEncoding.DecodeString(entry.Value)
		if err != nil {
			return ErrInvalidDEK
		}
		nonce, err := base64.StdEncoding.DecodeString(entry.Nonce)
		if err != nil {
			return ErrInvalidDEK
		}
		// Use a temporary LockedBuffer for the trial decryption so that we
		// do not cache a potentially invalid key.
		tempDEK := memguard.NewBufferFromBytes(dekBytes)
		_, decErr := DecryptAESGCM(tempDEK, ciphertext, nonce)
		tempDEK.Destroy()
		if decErr != nil {
			return ErrInvalidDEK
		}
		break // only need to verify one entry
	}

	// Validation passed (or vault is empty); cache the DEK.
	v.dek = memguard.NewBufferFromBytes(dekBytes)
	return nil
}

// ChangePassword re-encrypts the DEK under a new master password. The
// operation verifies the old password (by deriving the old KEK and decrypting
// the DEK), then generates a fresh salt, derives a new KEK, re-encrypts the
// same DEK with a fresh nonce, and atomically updates the vault file. Because
// only the KEK wrapping changes, all existing secret ciphertexts remain valid
// and no per-secret re-encryption is needed.
func (v *VaultManager) ChangePassword(oldPassword, newPassword *memguard.LockedBuffer) error {
	if newPassword.Size() < minPasswordLength {
		return ErrPasswordTooShort
	}

	// Step 1: Verify the old password by deriving the old KEK and decrypting the DEK.
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	oldSalt, err := base64.StdEncoding.DecodeString(vf.KDF.Salt)
	if err != nil {
		return fmt.Errorf("change password: decoding salt: %w", err)
	}

	oldParams := KDFParams{
		Algorithm:   vf.KDF.Algorithm,
		Memory:      vf.KDF.Memory,
		Iterations:  vf.KDF.Iterations,
		Parallelism: vf.KDF.Parallelism,
		Salt:        oldSalt,
	}

	oldKEK, err := DeriveKEK(oldPassword, oldParams)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	defer oldKEK.Destroy()

	encDEK, err := base64.StdEncoding.DecodeString(vf.DEK)
	if err != nil {
		return fmt.Errorf("change password: decoding DEK: %w", err)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(vf.DEKNonce)
	if err != nil {
		return fmt.Errorf("change password: decoding DEK nonce: %w", err)
	}

	dekBytes, err := DecryptAESGCM(oldKEK, encDEK, dekNonce)
	if err != nil {
		return ErrWrongPassword
	}

	// Step 2: Generate a fresh salt and derive a new KEK from the new password.
	newSalt, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	newParams := DefaultKDFParams()
	if v.kdfOverride != nil {
		newParams = *v.kdfOverride
	}
	newParams.Salt = newSalt

	newKEK, err := DeriveKEK(newPassword, newParams)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	defer newKEK.Destroy()

	// Step 3: Re-encrypt the same DEK with the new KEK and a fresh nonce.
	newEncDEK, newDEKNonce, err := EncryptAESGCM(newKEK, dekBytes)
	if err != nil {
		return fmt.Errorf("change password: encrypting DEK: %w", err)
	}

	// Step 4: Atomically update the vault file under a file lock. Re-read
	// inside the lock to avoid clobbering concurrent writes.
	return WithFileLock(v.path, func() error {
		vf, err := ReadVaultFile(v.path)
		if err != nil {
			return fmt.Errorf("change password: %w", err)
		}

		vf.KDF = KDFParamsJSON{
			Algorithm:   newParams.Algorithm,
			Memory:      newParams.Memory,
			Iterations:  newParams.Iterations,
			Parallelism: newParams.Parallelism,
			Salt:        base64.StdEncoding.EncodeToString(newSalt),
		}
		vf.DEK = base64.StdEncoding.EncodeToString(newEncDEK)
		vf.DEKNonce = base64.StdEncoding.EncodeToString(newDEKNonce)

		if err := WriteVaultFile(v.path, vf); err != nil {
			return fmt.Errorf("change password: %w", err)
		}

		v.params = newParams
		return nil
	})
}

// VaultKeyNames returns just the plaintext key names from the vault file,
// without loading or decrypting any secret values.
func (v *VaultManager) VaultKeyNames() ([]string, error) {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(vf.Keys))
	for k := range vf.Keys {
		keys = append(keys, k)
	}
	return keys, nil
}
