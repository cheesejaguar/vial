package vault

import (
	"bytes"
	"testing"

	"github.com/awnumar/memguard"
)

func TestDeriveKEKDeterministic(t *testing.T) {
	params := TestKDFParams()
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt: %v", err)
	}
	params.Salt = salt

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	kek1, err := DeriveKEK(password, params)
	if err != nil {
		t.Fatalf("DeriveKEK 1: %v", err)
	}
	defer kek1.Destroy()

	// Re-create password buffer since memguard may have wiped the first
	password2 := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password2.Destroy()

	kek2, err := DeriveKEK(password2, params)
	if err != nil {
		t.Fatalf("DeriveKEK 2: %v", err)
	}
	defer kek2.Destroy()

	if !bytes.Equal(kek1.Bytes(), kek2.Bytes()) {
		t.Error("same password + salt + params should produce the same KEK")
	}
}

func TestDeriveKEKDifferentSalts(t *testing.T) {
	params1 := TestKDFParams()
	salt1, _ := GenerateSalt()
	params1.Salt = salt1

	params2 := TestKDFParams()
	salt2, _ := GenerateSalt()
	params2.Salt = salt2

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	kek1, err := DeriveKEK(password, params1)
	if err != nil {
		t.Fatalf("DeriveKEK 1: %v", err)
	}
	defer kek1.Destroy()

	password2 := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password2.Destroy()

	kek2, err := DeriveKEK(password2, params2)
	if err != nil {
		t.Fatalf("DeriveKEK 2: %v", err)
	}
	defer kek2.Destroy()

	if bytes.Equal(kek1.Bytes(), kek2.Bytes()) {
		t.Error("different salts should produce different KEKs")
	}
}

func TestDeriveKEKDifferentPasswords(t *testing.T) {
	params := TestKDFParams()
	salt, _ := GenerateSalt()
	params.Salt = salt

	pw1 := memguard.NewBufferFromBytes([]byte("password-one-12"))
	defer pw1.Destroy()
	pw2 := memguard.NewBufferFromBytes([]byte("password-two-12"))
	defer pw2.Destroy()

	kek1, _ := DeriveKEK(pw1, params)
	defer kek1.Destroy()
	kek2, _ := DeriveKEK(pw2, params)
	defer kek2.Destroy()

	if bytes.Equal(kek1.Bytes(), kek2.Bytes()) {
		t.Error("different passwords should produce different KEKs")
	}
}

func TestDeriveKEKOutputSize(t *testing.T) {
	params := TestKDFParams()
	salt, _ := GenerateSalt()
	params.Salt = salt

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	kek, err := DeriveKEK(password, params)
	if err != nil {
		t.Fatalf("DeriveKEK: %v", err)
	}
	defer kek.Destroy()

	if kek.Size() != keySize {
		t.Errorf("KEK size = %d, want %d", kek.Size(), keySize)
	}
}

func TestDeriveKEKEmptySalt(t *testing.T) {
	params := TestKDFParams()
	// No salt set

	password := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer password.Destroy()

	_, err := DeriveKEK(password, params)
	if err == nil {
		t.Error("expected error for empty salt")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt: %v", err)
	}

	if len(salt) != saltSize {
		t.Errorf("salt size = %d, want %d", len(salt), saltSize)
	}

	// Two salts should differ
	salt2, _ := GenerateSalt()
	if bytes.Equal(salt, salt2) {
		t.Error("two generated salts should differ")
	}
}
