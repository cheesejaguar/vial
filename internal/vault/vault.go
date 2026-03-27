package vault

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/awnumar/memguard"
)

const minPasswordLength = 12

// VaultManager implements the Vault interface.
type VaultManager struct {
	path      string
	dek       *memguard.LockedBuffer // nil when locked
	params    KDFParams              // cached from vault file
	kdfOverride *KDFParams           // if set, used instead of DefaultKDFParams (for tests)
}

// NewVaultManager creates a new VaultManager for the vault at the given path.
func NewVaultManager(path string) *VaultManager {
	return &VaultManager{path: path}
}

// SetKDFParams overrides the default KDF parameters (use only for testing).
func (v *VaultManager) SetKDFParams(params KDFParams) {
	v.kdfOverride = &params
}

func (v *VaultManager) Path() string      { return v.path }
func (v *VaultManager) IsUnlocked() bool  { return v.dek != nil }
func (v *VaultManager) Version() int      { return 1 }

// Init creates a new vault file encrypted with the given master password.
func (v *VaultManager) Init(password *memguard.LockedBuffer) error {
	if password.Size() < minPasswordLength {
		return ErrPasswordTooShort
	}

	// Check if vault already exists
	if _, err := ReadVaultFile(v.path); err == nil {
		return ErrVaultExists
	}

	// Generate salt and derive KEK
	salt, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	params := DefaultKDFParams()
	if v.kdfOverride != nil {
		params = *v.kdfOverride
	}
	params.Salt = salt

	kek, err := DeriveKEK(password, params)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	defer kek.Destroy()

	// Generate DEK and encrypt it with KEK
	dek, err := GenerateDEK()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	encDEK, dekNonce, err := EncryptAESGCM(kek, dek.Bytes())
	if err != nil {
		dek.Destroy()
		return fmt.Errorf("init: encrypting DEK: %w", err)
	}

	// Build vault file
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

	// Keep vault unlocked after init
	v.dek = dek
	v.params = params
	return nil
}

// Unlock derives the KEK from the password, decrypts the DEK, and caches it.
func (v *VaultManager) Unlock(password *memguard.LockedBuffer) error {
	vf, err := ReadVaultFile(v.path)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(vf.KDF.Salt)
	if err != nil {
		return fmt.Errorf("unlock: decoding salt: %w", err)
	}

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
	defer kek.Destroy()

	encDEK, err := base64.StdEncoding.DecodeString(vf.DEK)
	if err != nil {
		return fmt.Errorf("unlock: decoding DEK: %w", err)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(vf.DEKNonce)
	if err != nil {
		return fmt.Errorf("unlock: decoding DEK nonce: %w", err)
	}

	dekBytes, err := DecryptAESGCM(kek, encDEK, dekNonce)
	if err != nil {
		return ErrWrongPassword
	}

	v.dek = memguard.NewBufferFromBytes(dekBytes)
	v.params = params
	return nil
}

// Lock zeroes and destroys the in-memory DEK.
func (v *VaultManager) Lock() {
	if v.dek != nil {
		v.dek.Destroy()
		v.dek = nil
	}
}

// SetSecret encrypts and stores a secret value in the vault.
func (v *VaultManager) SetSecret(key string, value *memguard.LockedBuffer) error {
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

		// Preserve metadata if updating an existing key
		if exists {
			entry.Aliases = existing.Aliases
			entry.Provider = existing.Provider
			entry.Tags = existing.Tags
			entry.Added = existing.Added // keep original add time
		}

		vf.Keys[key] = entry
		return WriteVaultFile(v.path, vf)
	})
	if err != nil {
		retErr = fmt.Errorf("set secret %q: %w", key, err)
	}
	return retErr
}

// GetSecret decrypts and returns a secret value.
// The caller owns the returned LockedBuffer and must call Destroy().
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

// ListSecrets returns all key names with metadata (no values).
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
				Aliases:  entry.Aliases,
				Provider: entry.Provider,
				Tags:     entry.Tags,
				Added:    added,
				Rotated:  rotated,
			},
		})
	}
	return infos
}

// RemoveSecret removes a key from the vault.
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

// GetMetadata returns metadata for a stored secret.
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
		Aliases:  entry.Aliases,
		Provider: entry.Provider,
		Tags:     entry.Tags,
		Added:    added,
		Rotated:  rotated,
	}, nil
}

// SetMetadata updates the metadata for a stored secret.
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
		vf.Keys[key] = entry
		return WriteVaultFile(v.path, vf)
	})
}

// DEKBytes returns the raw DEK bytes for session caching.
// The caller must not modify or free the returned bytes.
func (v *VaultManager) DEKBytes() []byte {
	if v.dek == nil {
		return nil
	}
	return v.dek.Bytes()
}

// UnlockWithDEK sets the DEK directly (used when restoring from session cache).
func (v *VaultManager) UnlockWithDEK(dekBytes []byte) error {
	// Verify the DEK works by trying to read the vault
	if _, err := ReadVaultFile(v.path); err != nil {
		return err
	}
	v.dek = memguard.NewBufferFromBytes(dekBytes)
	return nil
}

// VaultKeyNames returns just the key names from the vault file.
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
