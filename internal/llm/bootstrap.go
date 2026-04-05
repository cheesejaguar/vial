package llm

import (
	"fmt"

	"github.com/awnumar/memguard"
)

// VaultKeyGetter is a narrow interface for retrieving a secret value from the
// vault. It is used instead of importing the vault package directly to avoid a
// circular dependency (the vault package imports the config package, which
// would eventually import llm if we used the concrete type here).
//
// The returned [memguard.LockedBuffer] is owned by the caller, who must call
// Destroy on it after use.
type VaultKeyGetter interface {
	GetSecret(key string) (*memguard.LockedBuffer, error)
}

// Bootstrap constructs an LLM [Provider] whose API key is stored inside the
// vault itself. This keeps the LLM API key subject to the same encryption and
// access controls as every other secret in Vial.
//
// The function reads the API key from the vault using cfg.VaultKeyRef, copies
// it into a plain string, and immediately destroys the locked buffer. The
// plain string is then passed to the appropriate provider constructor. Callers
// must ensure the vault is unlocked before calling Bootstrap.
//
// Provider selection:
//   - "anthropic" → [AnthropicProvider]
//   - "openai", "openrouter", "" → [OpenAIProvider]
//   - anything else → [OpenAIProvider] (OpenAI-compatible default)
//
// Returns [ErrNoLLMConfigured] when cfg.VaultKeyRef is empty, signalling that
// the user has not configured an LLM provider.
func Bootstrap(vault VaultKeyGetter, cfg Config) (Provider, error) {
	if cfg.VaultKeyRef == "" {
		// No key reference means the feature is intentionally disabled;
		// callers should skip the LLM tier entirely rather than erroring.
		return nil, ErrNoLLMConfigured
	}

	keyBuf, err := vault.GetSecret(cfg.VaultKeyRef)
	if err != nil {
		return nil, fmt.Errorf("LLM API key %q not found in vault: %w", cfg.VaultKeyRef, err)
	}
	// Copy the key out of guarded memory before destroying the buffer.
	// The plain string is short-lived: it goes directly into the provider
	// struct and is not logged or written to disk.
	apiKey := string(keyBuf.Bytes())
	keyBuf.Destroy()

	switch cfg.Provider {
	case "anthropic":
		return NewAnthropicProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	case "openai", "openrouter", "":
		return NewOpenAIProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	default:
		// Treat any unrecognised provider name as an OpenAI-compatible
		// endpoint. This lets users point Vial at local LLM servers without
		// requiring explicit support for each one.
		return NewOpenAIProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	}
}
