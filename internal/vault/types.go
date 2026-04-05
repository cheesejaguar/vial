package vault

import (
	"time"

	"github.com/awnumar/memguard"
)

// VaultFile is the on-disk JSON representation of the vault. It follows a
// SOPS-style layout where key names in the Keys map are plaintext (enabling
// readable diffs and key enumeration without decryption) while values are
// individually AES-256-GCM encrypted. The file is stored at
// ~/.local/share/vial/vault.json with 0600 permissions.
type VaultFile struct {
	Version    int                    `json:"version"`
	KDF        KDFParamsJSON          `json:"kdf"`
	DEK        string                 `json:"dek"`       // base64-encoded encrypted DEK (wrapped by KEK)
	DEKNonce   string                 `json:"dek_nonce"` // base64-encoded nonce used when encrypting the DEK
	Keys       map[string]SecretEntry `json:"keys"`      // plaintext key name -> encrypted value + metadata
	AliasRules []AliasRule            `json:"alias_rules"`
	Projects   []ProjectRef           `json:"projects"`
}

// KDFParamsJSON is the JSON-serializable form of KDFParams, using a
// base64-encoded salt string instead of raw bytes. It is embedded in VaultFile
// so that unlock can reproduce the exact Argon2id derivation.
type KDFParamsJSON struct {
	Algorithm   string `json:"algorithm"`
	Memory      uint32 `json:"memory"`      // Argon2id memory cost in KiB
	Iterations  uint32 `json:"iterations"`   // number of Argon2id passes
	Parallelism uint8  `json:"parallelism"` // Argon2id lane count
	Salt        string `json:"salt"`         // base64-encoded random salt
}

// SecretEntry is a single encrypted secret stored in the vault file's Keys map.
// The Value and Nonce fields together form the AES-256-GCM ciphertext envelope
// for this secret, encrypted under the vault's DEK.
type SecretEntry struct {
	Value        string   `json:"value"`                   // base64(AES-GCM ciphertext + tag)
	Nonce        string   `json:"nonce"`                   // base64(12-byte GCM nonce)
	Aliases      []string `json:"aliases"`                 // user-defined alternative names for matching
	Provider     string   `json:"provider,omitempty"`      // service provider hint (e.g., "openai", "stripe")
	Tags         []string `json:"tags"`                    // free-form classification tags
	Added        string   `json:"added"`                   // RFC 3339 timestamp of first storage
	Rotated      string   `json:"rotated"`                 // RFC 3339 timestamp of most recent value update
	RotationDays int      `json:"rotation_days,omitempty"` // 0 = no rotation policy
}

// SecretMetadata holds the plaintext (non-secret) attributes of a stored
// secret. It is used by GetMetadata/SetMetadata and ListSecrets to expose
// organizational information without decrypting values.
type SecretMetadata struct {
	Aliases      []string  `json:"aliases,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Added        time.Time `json:"added"`
	Rotated      time.Time `json:"rotated"`
	RotationDays int       `json:"rotation_days,omitempty"`
}

// AliasRule defines a user-configured regex pattern that maps environment
// variable names to a canonical vault key. Used by the matching engine to
// resolve framework-prefixed or project-specific variable names.
type AliasRule struct {
	Pattern string `json:"pattern"` // regex matched against requested env var names
	MapsTo  string `json:"maps_to"` // canonical key name in the vault
}

// ProjectRef tracks a registered project directory that uses this vault.
type ProjectRef struct {
	Path       string `json:"path"`
	LastPoured string `json:"last_poured,omitempty"` // RFC 3339 timestamp of last "vial pour"
}

// Vault is the primary interface for vault operations. It abstracts the
// encryption lifecycle (init, unlock, lock) and CRUD operations on secrets.
// VaultManager is the concrete implementation. Methods that return a
// *memguard.LockedBuffer transfer ownership to the caller, who must call
// Destroy() on it.
type Vault interface {
	Init(password *memguard.LockedBuffer) error
	Unlock(password *memguard.LockedBuffer) error
	Lock()
	IsUnlocked() bool
	SetSecret(key string, value *memguard.LockedBuffer) error
	GetSecret(key string) (*memguard.LockedBuffer, error)
	ListSecrets() []SecretInfo
	RemoveSecret(key string) error
	GetMetadata(key string) (*SecretMetadata, error)
	SetMetadata(key string, meta SecretMetadata) error
	Path() string
	Version() int
}

// SecretInfo pairs a key name with its plaintext metadata for list responses.
// It never contains decrypted secret values.
type SecretInfo struct {
	Key      string
	Metadata SecretMetadata
}
