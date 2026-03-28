package share

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
)

// Bundle is an encrypted, time-limited secret bundle for sharing.
type Bundle struct {
	Version    int    `json:"version"`
	Encrypted  string `json:"encrypted"` // base64-encoded AES-GCM ciphertext
	Nonce      string `json:"nonce"`     // base64-encoded GCM nonce
	Salt       string `json:"salt"`      // base64-encoded argon2 salt
	ExpiresAt  string `json:"expires_at"`
	KeyCount   int    `json:"key_count"`
}

// BundlePayload is the plaintext content of a bundle.
type BundlePayload struct {
	Secrets   map[string]string `json:"secrets"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// CreateBundle encrypts the given secrets with a passphrase and optional expiry.
func CreateBundle(secrets map[string]string, passphrase string, expiry time.Duration) (*Bundle, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(expiry)

	payload := BundlePayload{
		Secrets:   secrets,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	// Derive key from passphrase
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)

	// Encrypt with AES-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &Bundle{
		Version:   1,
		Encrypted: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Salt:      base64.StdEncoding.EncodeToString(salt),
		ExpiresAt: expiresAt.Format(time.RFC3339),
		KeyCount:  len(secrets),
	}, nil
}

// OpenBundle decrypts a bundle with the given passphrase.
func OpenBundle(bundle *Bundle, passphrase string) (*BundlePayload, error) {
	// Check expiry
	expiresAt, err := time.Parse(time.RFC3339, bundle.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parsing expiry: %w", err)
	}
	if time.Now().UTC().After(expiresAt) {
		return nil, fmt.Errorf("bundle expired at %s", bundle.ExpiresAt)
	}

	// Decode components
	salt, err := base64.StdEncoding.DecodeString(bundle.Salt)
	if err != nil {
		return nil, fmt.Errorf("decoding salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(bundle.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(bundle.Encrypted)
	if err != nil {
		return nil, fmt.Errorf("decoding ciphertext: %w", err)
	}

	// Derive key
	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)

	// Decrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong passphrase?)")
	}

	var payload BundlePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("parsing payload: %w", err)
	}

	return &payload, nil
}

// Marshal serializes a bundle to JSON.
func (b *Bundle) Marshal() ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

// UnmarshalBundle deserializes a bundle from JSON.
func UnmarshalBundle(data []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing bundle: %w", err)
	}
	if b.Version != 1 {
		return nil, fmt.Errorf("unsupported bundle version: %d", b.Version)
	}
	return &b, nil
}
