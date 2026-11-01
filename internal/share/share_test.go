package share

import (
	"strings"
	"testing"
	"time"
)

// TestCreateAndOpenBundle verifies the full round-trip: encrypt secrets into a
// bundle and then decrypt them back with the correct passphrase.
func TestCreateAndOpenBundle(t *testing.T) {
	secrets := map[string]string{
		"OPENAI_API_KEY":    "sk-abc123",
		"STRIPE_SECRET_KEY": "sk_live_xyz",
	}

	bundle, err := CreateBundle(secrets, "test-passphrase", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if bundle.Version != 1 {
		t.Errorf("expected version 1, got %d", bundle.Version)
	}
	if bundle.KeyCount != 2 {
		t.Errorf("expected key count 2, got %d", bundle.KeyCount)
	}

	// Open with correct passphrase
	payload, err := OpenBundle(bundle, "test-passphrase")
	if err != nil {
		t.Fatal(err)
	}

	if len(payload.Secrets) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(payload.Secrets))
	}
	if payload.Secrets["OPENAI_API_KEY"] != "sk-abc123" {
		t.Errorf("wrong value for OPENAI_API_KEY")
	}
	if payload.Secrets["STRIPE_SECRET_KEY"] != "sk_live_xyz" {
		t.Errorf("wrong value for STRIPE_SECRET_KEY")
	}
}

// TestOpenBundleWrongPassphrase confirms that AES-GCM authentication rejects
// a bundle opened with the wrong passphrase.
func TestOpenBundleWrongPassphrase(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, err := CreateBundle(secrets, "correct", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := OpenBundle(bundle, "wrong")
	if err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
	if !strings.Contains(err.Error(), "decryption failed") {
		t.Errorf("err = %q, want a decryption failure", err)
	}
	if payload != nil {
		t.Error("no payload should be returned on failure")
	}
}

// TestOpenBundleExpired verifies that a bundle created with a negative duration
// (already in the past) is rejected before attempting decryption.
func TestOpenBundleExpired(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, err := CreateBundle(secrets, "pass", -1*time.Hour) // already expired
	if err != nil {
		t.Fatal(err)
	}

	_, err = OpenBundle(bundle, "pass")
	if err == nil {
		t.Fatal("expected error for expired bundle")
	}
	// The plaintext expiry check must reject it before decryption is tried.
	if !strings.HasPrefix(err.Error(), "bundle expired at ") || strings.Contains(err.Error(), "verified from encrypted payload") {
		t.Errorf("err = %q, want the pre-decryption expiry error", err)
	}
}

// TestMarshalUnmarshal verifies that a bundle survives a JSON serialization
// round-trip and that the decrypted secrets are still correct afterwards.
func TestMarshalUnmarshal(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, err := CreateBundle(secrets, "pass", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	data, err := bundle.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := UnmarshalBundle(data)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Version != bundle.Version {
		t.Error("version mismatch after marshal/unmarshal")
	}
	if parsed.KeyCount != bundle.KeyCount {
		t.Error("key count mismatch")
	}

	// Verify we can still decrypt after round-tripping through JSON.
	payload, err := OpenBundle(parsed, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Secrets["KEY"] != "value" {
		t.Error("value mismatch after marshal/unmarshal")
	}
}
