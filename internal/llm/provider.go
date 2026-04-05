// Package llm provides a thin abstraction over LLM inference APIs used by
// Vial's Tier 5 matching engine. The package defines a single [Provider]
// interface and ships two concrete implementations: [AnthropicProvider] for
// the Anthropic Messages API and [OpenAIProvider] for any OpenAI-compatible
// endpoint (OpenAI, OpenRouter, local LLMs, etc.).
//
// # Role in the matcher pipeline
//
// The LLM tier is the last resort in the five-tier matching chain. It is
// invoked only when all deterministic tiers (exact, normalize, alias,
// comment) fail to reach confidence >= 0.9. Results from this tier are
// capped at 0.75 so they can never outrank a deterministic match.
//
// # Fail-open contract
//
// If the provider returns an error the caller treats it as a non-match and
// continues without the secret. This keeps the CLI usable even when the
// configured LLM is unreachable or rate-limited.
//
// # Hallucination guard
//
// After the LLM suggests a vault key name the caller MUST verify that the
// name actually exists in the vault before using it. This is enforced in
// the matcher tier, not here, but it is an invariant that every caller of
// [Provider.Complete] is expected to uphold.
//
// # Key material
//
// API keys are loaded from the vault at bootstrap time via [Bootstrap] and
// held in a plain string for the lifetime of the provider. The
// [VaultKeyGetter] interface is used instead of a direct vault import to
// avoid a circular dependency.
package llm

import (
	"context"
	"errors"
)

// Sentinel errors returned by providers and [Bootstrap].
var (
	// ErrNoLLMConfigured is returned by [Bootstrap] when the configuration
	// does not specify a vault key reference, meaning the user has not set
	// up an LLM provider.
	ErrNoLLMConfigured = errors.New("no LLM provider configured")

	// ErrProviderError wraps transport-level or API-level failures from a
	// remote LLM provider. Callers should treat this as a transient error
	// and fail open rather than blocking the user.
	ErrProviderError = errors.New("LLM provider error")
)

// Provider is the interface all LLM backends must implement. A Provider is
// safe to call concurrently; each [Complete] call is independent.
type Provider interface {
	// Complete sends a prompt to the LLM and returns its response. ctx can
	// be used to enforce a deadline — recommended to keep latency bounded
	// in interactive flows.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// Name returns a short identifier for the backend (e.g. "openai",
	// "anthropic"). Used in log messages and config validation.
	Name() string

	// Available reports whether the provider has the minimum configuration
	// needed to attempt a call (i.e. a non-empty API key). A false return
	// means Complete will always fail.
	Available() bool
}

// CompletionRequest holds all parameters for a single LLM inference call.
// Zero values for numeric fields are replaced with safe defaults inside each
// provider implementation.
type CompletionRequest struct {
	SystemPrompt string  // optional system/instruction context
	UserPrompt   string  // the main user turn sent to the model
	MaxTokens    int     // 0 → provider default (usually 200)
	Temperature  float64 // 0.0 → deterministic; higher → more creative
}

// CompletionResponse is the structured result of a successful [Provider.Complete]
// call. Callers should check [Provider.Available] before interpreting this.
type CompletionResponse struct {
	Content      string // raw text returned by the model
	TokensUsed   int    // total tokens consumed (input + output) for cost tracking
	Model        string // model identifier echoed back by the API
	FinishReason string // e.g. "end_turn", "stop", "length"; provider-specific
}

// Config carries the user-facing LLM configuration loaded from
// ~/.config/vial/config.yaml via Viper. It is passed to [Bootstrap] which
// resolves the API key from the vault and constructs the appropriate
// [Provider].
type Config struct {
	// Provider selects the backend: "openai", "anthropic", "openrouter", or
	// any string that implies an OpenAI-compatible API.
	Provider string `mapstructure:"provider" yaml:"provider"`

	// Endpoint overrides the base URL. Leave empty to use each provider's
	// default public endpoint.
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`

	// Model is the model identifier forwarded verbatim to the API (e.g.
	// "gpt-4o-mini", "claude-sonnet-4-6"). Leave empty for the provider's
	// built-in default.
	Model string `mapstructure:"model" yaml:"model"`

	// VaultKeyRef is the vault key whose plaintext value is the LLM API
	// key. The value is retrieved from the vault at bootstrap and never
	// written to disk.
	VaultKeyRef string `mapstructure:"vault_key_ref" yaml:"vault_key_ref"`
}
