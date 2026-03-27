package vault

import "errors"

var (
	ErrVaultExists      = errors.New("vault already exists")
	ErrVaultNotFound    = errors.New("vault file not found")
	ErrVaultLocked      = errors.New("vault is locked; run 'vial uncork' first")
	ErrWrongPassword    = errors.New("incorrect master password")
	ErrSecretNotFound   = errors.New("secret not found")
	ErrPasswordTooShort = errors.New("password must be at least 12 characters")
	ErrVaultCorrupted   = errors.New("vault file is corrupted or tampered with")
	ErrValueTooLarge    = errors.New("secret value exceeds 1 MiB limit")
)

const maxValueSize = 1 << 20 // 1 MiB

// MaxValueSizeExported returns the max value size for external use.
func MaxValueSizeExported() int { return maxValueSize }
