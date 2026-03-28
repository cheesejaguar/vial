package vault

import (
	"crypto/rand"
	"fmt"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/argon2"
)

const (
	defaultMemory      = 64 * 1024 // 64 MiB in KiB
	defaultIterations  = 3
	defaultParallelism = 1
	saltSize           = 16
)

// KDFParams holds the Argon2id parameters for key derivation.
type KDFParams struct {
	Algorithm   string `json:"algorithm"`
	Memory      uint32 `json:"memory"` // in KiB
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        []byte `json:"salt"`
}

// DefaultKDFParams returns the recommended Argon2id parameters.
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		Memory:      defaultMemory,
		Iterations:  defaultIterations,
		Parallelism: defaultParallelism,
	}
}

// TestKDFParams returns fast parameters for use in tests only.
func TestKDFParams() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		Memory:      1024, // 1 MiB
		Iterations:  1,
		Parallelism: 1,
	}
}

// GenerateSalt creates a cryptographically random 16-byte salt.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return salt, nil
}

// DeriveKEK derives a 256-bit Key Encryption Key from the master password using Argon2id.
// The result is stored in a memguard LockedBuffer. The caller owns the buffer and must Destroy() it.
func DeriveKEK(password *memguard.LockedBuffer, params KDFParams) (*memguard.LockedBuffer, error) {
	if len(params.Salt) == 0 {
		return nil, fmt.Errorf("salt must not be empty")
	}

	derived := argon2.IDKey(
		password.Bytes(),
		params.Salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		keySize,
	)

	lb := memguard.NewBufferFromBytes(derived)
	return lb, nil
}
