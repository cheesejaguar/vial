// Package share implements passphrase-encrypted, time-limited secret bundles
// that let a vault owner safely hand a subset of secrets to another person or
// machine without exposing the master password or the full vault.
//
// A Bundle is a self-contained JSON document containing:
//   - an Argon2id-derived key (random 16-byte salt, 3 passes, 64 MiB memory)
//   - the payload encrypted with AES-256-GCM (random nonce per bundle)
//   - the expiry time in the outer envelope (for fast pre-decryption rejection)
//   - the expiry time inside the authenticated payload (tamper-evident verification)
//
// The double-expiry check means a recipient cannot extend the bundle lifetime by
// editing the outer JSON: the inner expiry is authenticated by the GCM tag and
// will fail to decrypt after the original window closes.
//
// Typical usage:
//
//	bundle, _ := share.CreateBundle(secrets, "one-time-passphrase", 24*time.Hour)
//	data, _   := bundle.Marshal()     // send data to recipient
//	// --- recipient side ---
//	b, _      := share.UnmarshalBundle(data)
//	payload, _ := share.OpenBundle(b, "one-time-passphrase")
//	// payload.Secrets now contains the plaintext key→value map
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
// All fields are safe to transmit or store in plaintext — the sensitive data
// is inside the Encrypted ciphertext, authenticated by AES-GCM.
type Bundle struct {
	Version   int    `json:"version"`             // schema version; currently always 1
	Encrypted string `json:"encrypted"`           // base64-encoded AES-GCM ciphertext
	Nonce     string `json:"nonce"`               // base64-encoded 12-byte GCM nonce
	Salt      string `json:"salt"`                // base64-encoded 16-byte Argon2id salt
	ExpiresAt string `json:"expires_at"`          // RFC3339 expiry for fast outer rejection
	KeyCount  int    `json:"key_count"`           // number of secrets included (informational)
}

// BundlePayload is the plaintext structure sealed inside a Bundle.
// It is marshaled to JSON, encrypted, and authenticated so that every field
// is tamper-evident.
type BundlePayload struct {
	Secrets   map[string]string `json:"secrets"`    // key name → plaintext value
	CreatedAt time.Time         `json:"created_at"` // when the bundle was created (UTC)
	ExpiresAt time.Time         `json:"expires_at"` // authoritative expiry inside ciphertext
}

// CreateBundle encrypts secrets under passphrase and returns a Bundle valid for expiry duration.
// A fresh random salt and nonce are generated for each call so two bundles
// created from the same secrets and passphrase are always ciphertext-distinct.
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

	// Derive a 256-bit AES key from the passphrase using Argon2id.
	// Parameters match the main vault KDF for consistent security posture.
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)

	// Encrypt with AES-256-GCM.
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

// OpenBundle decrypts and authenticates bundle using passphrase.
// It enforces expiry at two levels:
//  1. The outer ExpiresAt field is checked first for a cheap early exit.
//  2. The ExpiresAt inside the authenticated payload is checked after
//     decryption, preventing an attacker from extending the window by
//     modifying the outer envelope.
//
// Returns an error if the passphrase is wrong, the bundle is expired, or
// any authenticated field has been tampered with.
func OpenBundle(bundle *Bundle, passphrase string) (*BundlePayload, error) {
	// Quick outer expiry check — avoids the expensive Argon2id derivation for
	// bundles that are obviously stale.
	expiresAt, err := time.Parse(time.RFC3339, bundle.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parsing expiry: %w", err)
	}
	if time.Now().UTC().After(expiresAt) {
		return nil, fmt.Errorf("bundle expired at %s", bundle.ExpiresAt)
	}

	// Decode the base64-encoded cryptographic components.
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

	// Re-derive the key from the passphrase and the stored salt.
	key := argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	// gcm.Open authenticates and decrypts in one step; an error here means
	// either a wrong passphrase or ciphertext corruption/tampering.
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong passphrase?)")
	}

	var payload BundlePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("parsing payload: %w", err)
	}

	// Secondary expiry check against the authenticated inner timestamp.
	// This prevents an attacker who has the outer JSON from extending the
	// bundle's claimed expiry — the inner value is protected by the GCM tag.
	if !payload.ExpiresAt.IsZero() && time.Now().UTC().After(payload.ExpiresAt) {
		return nil, fmt.Errorf("bundle expired at %s (verified from encrypted payload)", payload.ExpiresAt.Format(time.RFC3339))
	}

	return &payload, nil
}

// Marshal serializes the Bundle to indented JSON suitable for display or
// writing to a file.
func (b *Bundle) Marshal() ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

// UnmarshalBundle deserializes a Bundle from JSON and validates that its
// schema version is supported.  Returns an error for unknown versions so
// that older clients fail clearly rather than silently misinterpreting the
// ciphertext layout.
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
