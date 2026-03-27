package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/awnumar/memguard"
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // GCM standard nonce
)

// GenerateDEK creates a random 256-bit data encryption key in a guarded buffer.
func GenerateDEK() (*memguard.LockedBuffer, error) {
	buf := make([]byte, keySize)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generating DEK: %w", err)
	}
	lb := memguard.NewBufferFromBytes(buf)
	return lb, nil
}

// GenerateNonce creates a random 12-byte GCM nonce.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return nonce, nil
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the given key.
// Returns ciphertext and nonce. A fresh random nonce is generated per call.
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

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts ciphertext using AES-256-GCM with the given key and nonce.
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
