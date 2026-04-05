package vault

import (
	"crypto/rand"
	"fmt"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/argon2"
)

// Production Argon2id cost parameters. These are tuned so that key derivation
// takes approximately 0.5-1 second on modern hardware, making brute-force
// attacks against the master password computationally expensive.
const (
	defaultMemory      = 64 * 1024 // 64 MiB expressed in KiB (Argon2id memory unit)
	defaultIterations  = 3         // number of Argon2id passes over memory
	defaultParallelism = 1         // single-threaded to keep memory usage predictable
	saltSize           = 16        // 128-bit random salt per vault
)

// KDFParams holds the Argon2id parameters used to derive the KEK from the
// master password. These are persisted in the vault file so that the same
// derivation can be reproduced at unlock time, even if defaults change in
// future versions.
type KDFParams struct {
	Algorithm   string `json:"algorithm"`
	Memory      uint32 `json:"memory"` // in KiB
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        []byte `json:"salt"`
}

// DefaultKDFParams returns the recommended production Argon2id parameters
// (64 MiB memory, 3 iterations). These are used when creating a new vault or
// changing the master password.
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		Memory:      defaultMemory,
		Iterations:  defaultIterations,
		Parallelism: defaultParallelism,
	}
}

// TestKDFParams returns fast, low-cost Argon2id parameters (1 MiB, 1 iteration)
// suitable only for tests. Production params would make each test case take
// 15+ seconds. Always call vm.SetKDFParams(vault.TestKDFParams()) in test setup.
func TestKDFParams() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		Memory:      1024, // 1 MiB -- fast enough for CI
		Iterations:  1,
		Parallelism: 1,
	}
}

// GenerateSalt creates a cryptographically random 128-bit (16-byte) salt from
// crypto/rand. A unique salt is generated for each vault initialization and
// password change to ensure that identical passwords produce different KEKs.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return salt, nil
}

// DeriveKEK derives a 256-bit Key Encryption Key from the master password
// using Argon2id with the given parameters and salt. The result is placed in a
// memguard LockedBuffer (mlock'd, guard-paged memory). The caller owns the
// returned buffer and must call Destroy() when done. The password buffer is
// borrowed, not consumed.
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
		keySize, // 32 bytes = AES-256
	)

	// NewBufferFromBytes copies into mlock'd memory and zeroes the source slice.
	lb := memguard.NewBufferFromBytes(derived)
	return lb, nil
}
