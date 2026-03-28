package llm

import (
	"context"
	"errors"
)

var (
	ErrNoLLMConfigured = errors.New("no LLM provider configured")
	ErrProviderError   = errors.New("LLM provider error")
)

// Provider is the interface all LLM backends implement.
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Name() string
	Available() bool
}

// CompletionRequest holds the parameters for an LLM completion.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// CompletionResponse holds the LLM's response.
type CompletionResponse struct {
	Content      string
	TokensUsed   int
	Model        string
	FinishReason string
}

// Config holds LLM provider configuration.
type Config struct {
	Provider    string `mapstructure:"provider" yaml:"provider"`           // "openai", "anthropic", "openrouter"
	Endpoint    string `mapstructure:"endpoint" yaml:"endpoint"`           // base URL
	Model       string `mapstructure:"model" yaml:"model"`                 // model identifier
	VaultKeyRef string `mapstructure:"vault_key_ref" yaml:"vault_key_ref"` // vault key holding the API key
}
