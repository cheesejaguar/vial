package vault

import (
	"bytes"
	"testing"

	"github.com/awnumar/memguard"
)

// TestEncryptDecryptRoundTrip verifies AES-GCM encrypt-then-decrypt for various
// payload types: short, empty, long, and binary data.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	defer key.Destroy()

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"short", []byte("sk-proj-abc123")},
		{"empty", []byte("")},
		{"long", bytes.Repeat([]byte("x"), 10000)},
		{"binary", []byte{0x00, 0x01, 0xff, 0xfe}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, nonce, err := EncryptAESGCM(key, tt.plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}

			decrypted, err := DecryptAESGCM(key, ciphertext, nonce)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("decrypted != plaintext: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

// TestEncryptProducesUniqueNonces confirms that two encryptions of the same
// plaintext produce different nonces (nonce reuse would break GCM security).
func TestEncryptProducesUniqueNonces(t *testing.T) {
	key, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	defer key.Destroy()

	plaintext := []byte("same-plaintext")

	_, nonce1, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}

	_, nonce2, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}

	if bytes.Equal(nonce1, nonce2) {
		t.Error("two encryptions produced the same nonce")
	}
}

// TestEncryptProducesUniqueCiphertexts confirms that identical plaintexts yield
// different ciphertexts due to random nonces.
func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	key, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	defer key.Destroy()

	plaintext := []byte("same-plaintext")

	ct1, _, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}

	ct2, _, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same plaintext produced the same ciphertext")
	}
}

// TestDecryptWithWrongKey verifies that GCM authentication rejects decryption
// with a different key.
func TestDecryptWithWrongKey(t *testing.T) {
	key1, _ := GenerateDEK()
	defer key1.Destroy()
	key2, _ := GenerateDEK()
	defer key2.Destroy()

	ciphertext, nonce, err := EncryptAESGCM(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = DecryptAESGCM(key2, ciphertext, nonce)
	if err == nil {
		t.Error("expected decryption with wrong key to fail")
	}
}

// TestDecryptTamperedCiphertext verifies that GCM detects a single flipped bit
// in the ciphertext.
func TestDecryptTamperedCiphertext(t *testing.T) {
	key, _ := GenerateDEK()
	defer key.Destroy()

	ciphertext, nonce, err := EncryptAESGCM(key, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Flip a bit in the ciphertext
	ciphertext[0] ^= 0x01

	_, err = DecryptAESGCM(key, ciphertext, nonce)
	if err == nil {
		t.Error("expected decryption of tampered ciphertext to fail")
	}
}

// TestEncryptInvalidKeySize verifies that EncryptAESGCM rejects keys that are
// not exactly 32 bytes.
func TestEncryptInvalidKeySize(t *testing.T) {
	shortKey := memguard.NewBufferFromBytes([]byte("too-short"))
	defer shortKey.Destroy()

	_, _, err := EncryptAESGCM(shortKey, []byte("test"))
	if err == nil {
		t.Error("expected error for invalid key size")
	}
}

// TestGenerateDEK verifies that a generated DEK has the correct 32-byte size.
func TestGenerateDEK(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	defer dek.Destroy()

	if dek.Size() != keySize {
		t.Errorf("DEK size = %d, want %d", dek.Size(), keySize)
	}
}

// TestGenerateNonce verifies that a generated nonce has the correct 12-byte size.
func TestGenerateNonce(t *testing.T) {
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}

	if len(nonce) != nonceSize {
		t.Errorf("nonce size = %d, want %d", len(nonce), nonceSize)
	}
}
