package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/awnumar/memguard"
)

const (
	keySize   = 32 // AES-256 requires a 256-bit (32-byte) key
	nonceSize = 12 // GCM standard nonce size (96 bits)
)

// GenerateDEK creates a random 256-bit data encryption key in a memguard
// LockedBuffer (mlock'd, guard-paged). The returned buffer is owned by the
// caller and must be destroyed when no longer needed. The random bytes come
// from crypto/rand.
func GenerateDEK() (*memguard.LockedBuffer, error) {
	buf := make([]byte, keySize)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generating DEK: %w", err)
	}
	// NewBufferFromBytes copies into mlock'd memory and zeroes the source slice.
	lb := memguard.NewBufferFromBytes(buf)
	return lb, nil
}

// GenerateNonce creates a random 12-byte (96-bit) nonce from crypto/rand for
// use with AES-GCM. A fresh nonce must be generated for every encryption
// operation; nonce reuse under the same key is catastrophic for GCM security.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return nonce, nil
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the given key.
// A fresh random nonce is generated from crypto/rand on every call -- callers
// must never supply or derive nonces deterministically. The returned ciphertext
// includes the GCM authentication tag (appended by Seal). The key buffer is
// borrowed, not consumed; the caller retains ownership.
func EncryptAESGCM(key *memguard.LockedBuffer, plaintext []byte) (ciphertext, nonce []byte, err error) {
	if key.Size() != keySize {
		return nil, nil, fmt.Errorf("key must be %d bytes, got %d", keySize, key.Size())
	}

	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce, err = GenerateNonce()
	if err != nil {
		return nil, nil, err
	}

	// Seal appends the ciphertext and GCM tag to the nil dst slice.
	// No additional authenticated data (AAD) is used.
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts and authenticates ciphertext using AES-256-GCM with
// the given key and nonce. The GCM tag is verified during Open; any tampering
// with the ciphertext, nonce, or use of a wrong key results in an error. The
// key buffer is borrowed, not consumed.
func DecryptAESGCM(key *memguard.LockedBuffer, ciphertext, nonce []byte) ([]byte, error) {
	if key.Size() != keySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", keySize, key.Size())
	}

	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
