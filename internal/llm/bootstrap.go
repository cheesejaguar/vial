package llm

import (
	"fmt"

	"github.com/awnumar/memguard"
)

// VaultKeyGetter is an interface for retrieving secrets from the vault.
// This avoids a circular dependency on the vault package.
type VaultKeyGetter interface {
	GetSecret(key string) (*memguard.LockedBuffer, error)
}

// Bootstrap creates an LLM Provider using a key stored in the vault itself.
func Bootstrap(vault VaultKeyGetter, cfg Config) (Provider, error) {
	if cfg.VaultKeyRef == "" {
		return nil, ErrNoLLMConfigured
	}

	keyBuf, err := vault.GetSecret(cfg.VaultKeyRef)
	if err != nil {
		return nil, fmt.Errorf("LLM API key %q not found in vault: %w", cfg.VaultKeyRef, err)
	}
	apiKey := string(keyBuf.Bytes())
	keyBuf.Destroy()

	switch cfg.Provider {
	case "anthropic":
		return NewAnthropicProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	case "openai", "openrouter", "":
		return NewOpenAIProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	default:
		// Assume OpenAI-compatible for unknown providers
		return NewOpenAIProvider(cfg.Endpoint, apiKey, cfg.Model), nil
	}
}
