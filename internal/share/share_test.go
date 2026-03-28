package share

import (
	"testing"
	"time"
)

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

func TestOpenBundleWrongPassphrase(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, _ := CreateBundle(secrets, "correct", 24*time.Hour)

	_, err := OpenBundle(bundle, "wrong")
	if err == nil {
		t.Error("expected error with wrong passphrase")
	}
}

func TestOpenBundleExpired(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, _ := CreateBundle(secrets, "pass", -1*time.Hour) // already expired

	_, err := OpenBundle(bundle, "pass")
	if err == nil {
		t.Error("expected error for expired bundle")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	secrets := map[string]string{"KEY": "value"}
	bundle, _ := CreateBundle(secrets, "pass", 24*time.Hour)

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

	// Verify we can still decrypt
	payload, err := OpenBundle(parsed, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Secrets["KEY"] != "value" {
		t.Error("value mismatch after marshal/unmarshal")
	}
}
