package vault

import (
	"time"

	"github.com/awnumar/memguard"
)

// VaultFile is the on-disk JSON representation of the vault.
type VaultFile struct {
	Version  int                    `json:"version"`
	KDF      KDFParamsJSON          `json:"kdf"`
	DEK      string                 `json:"dek"`       // base64-encoded encrypted DEK
	DEKNonce string                 `json:"dek_nonce"` // base64-encoded nonce for DEK encryption
	Keys     map[string]SecretEntry `json:"keys"`
	AliasRules []AliasRule          `json:"alias_rules"`
	Projects   []ProjectRef         `json:"projects"`
}

// KDFParamsJSON is the JSON-serializable form of KDFParams.
type KDFParamsJSON struct {
	Algorithm   string `json:"algorithm"`
	Memory      uint32 `json:"memory"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        string `json:"salt"` // base64-encoded
}

// SecretEntry is a single encrypted secret in the vault file.
type SecretEntry struct {
	Value    string   `json:"value"`   // base64-encoded encrypted value
	Nonce    string   `json:"nonce"`   // base64-encoded GCM nonce
	Aliases  []string `json:"aliases"`
	Provider string   `json:"provider,omitempty"`
	Tags     []string `json:"tags"`
	Added    string   `json:"added"`
	Rotated  string   `json:"rotated"`
}

// SecretMetadata holds non-secret data about a secret.
type SecretMetadata struct {
	Aliases  []string  `json:"aliases,omitempty"`
	Provider string    `json:"provider,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Added    time.Time `json:"added"`
	Rotated  time.Time `json:"rotated"`
}

// AliasRule defines a regex pattern mapping to a canonical key name (Phase 2+).
type AliasRule struct {
	Pattern string `json:"pattern"`
	MapsTo  string `json:"maps_to"`
}

// ProjectRef tracks a registered project directory (Phase 2+).
type ProjectRef struct {
	Path       string `json:"path"`
	LastPoured string `json:"last_poured,omitempty"`
}

// Vault is the primary interface for vault operations.
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

// SecretInfo is a summary of a stored secret (no value).
type SecretInfo struct {
	Key      string
	Metadata SecretMetadata
}
