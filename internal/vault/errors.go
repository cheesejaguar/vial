package vault

import "errors"

// Sentinel errors returned by vault operations. These are stable values that
// callers (CLI, MCP server, dashboard API) can compare with errors.Is.
var (
	ErrVaultExists      = errors.New("vault already exists")
	ErrVaultNotFound    = errors.New("vault file not found")
	ErrVaultLocked      = errors.New("vault is locked; run 'vial uncork' first")
	ErrWrongPassword    = errors.New("incorrect master password")
	ErrSecretNotFound   = errors.New("secret not found")
	ErrPasswordTooShort = errors.New("password must be at least 12 characters")
	ErrVaultCorrupted   = errors.New("vault file is corrupted or tampered with")
	ErrValueTooLarge    = errors.New("secret value exceeds 1 MiB limit")
	ErrInvalidDEK       = errors.New("cached DEK failed to decrypt vault data")
	ErrInvalidKeyName   = errors.New("invalid key name: must match [A-Za-z_][A-Za-z0-9_]* and be at most 256 characters")
)

// maxValueSize caps individual secret values at 1 MiB to prevent accidental
// storage of large blobs (e.g., binary files) in the JSON vault.
const maxValueSize = 1 << 20 // 1 MiB

// MaxValueSizeExported returns the maximum allowed secret value size in bytes.
// It is exported for use in CLI validation messages.
func MaxValueSizeExported() int { return maxValueSize }
