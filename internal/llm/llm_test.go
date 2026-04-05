package llm

import (
	"strings"
	"testing"

	"github.com/awnumar/memguard"
)

// TestParseMatchResponse exercises the full surface of ParseMatchResponse,
// including plain JSON, markdown-fenced JSON, whitespace padding, and
// out-of-range confidence values.
func TestParseMatchResponse(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantMatch string
		wantConf  float64
		wantErr   bool
	}{
		{
			name:      "valid match",
			raw:       `{"match": "OPENAI_API_KEY", "confidence": 0.9, "reason": "common variant"}`,
			wantMatch: "OPENAI_API_KEY",
			wantConf:  0.9,
		},
		{
			name:      "no match",
			raw:       `{"match": "NO_MATCH", "confidence": 0.0, "reason": "no suitable match"}`,
			wantMatch: "NO_MATCH",
			wantConf:  0.0,
		},
		{
			// Models frequently wrap JSON in ```json fences despite instructions;
			// verify that the fence-stripping path handles the language tag.
			name:      "markdown code fence",
			raw:       "```json\n{\"match\": \"STRIPE_KEY\", \"confidence\": 0.8, \"reason\": \"stripe key\"}\n```",
			wantMatch: "STRIPE_KEY",
			wantConf:  0.8,
		},
		{
			// Some models use bare ``` without a language tag.
			name:      "code fence without language",
			raw:       "```\n{\"match\": \"KEY\", \"confidence\": 0.5, \"reason\": \"test\"}\n```",
			wantMatch: "KEY",
			wantConf:  0.5,
		},
		{
			name:    "invalid json",
			raw:     "not json at all",
			wantErr: true,
		},
		{
			// Confidence > 1.0 should be rejected; LLMs occasionally hallucinate
			// values like 1.5 when they are "very confident".
			name:    "invalid confidence too high",
			raw:     `{"match": "KEY", "confidence": 1.5, "reason": "test"}`,
			wantErr: true,
		},
		{
			name:    "invalid confidence negative",
			raw:     `{"match": "KEY", "confidence": -0.1, "reason": "test"}`,
			wantErr: true,
		},
		{
			name:      "whitespace padded",
			raw:       `  {"match": "KEY", "confidence": 0.7, "reason": "test"}  `,
			wantMatch: "KEY",
			wantConf:  0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseMatchResponse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Match != tt.wantMatch {
				t.Errorf("Match = %q, want %q", resp.Match, tt.wantMatch)
			}
			if resp.Confidence != tt.wantConf {
				t.Errorf("Confidence = %f, want %f", resp.Confidence, tt.wantConf)
			}
		})
	}
}

// TestFormatMatchPrompt verifies that the formatted prompt includes both the
// requested key and all vault keys.
func TestFormatMatchPrompt(t *testing.T) {
	prompt := FormatMatchPrompt("MY_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	if !strings.Contains(prompt, "MY_KEY") {
		t.Error("prompt should contain requested key")
	}
	if !strings.Contains(prompt, "OPENAI_API_KEY") {
		t.Error("prompt should contain vault keys")
	}
	if !strings.Contains(prompt, "STRIPE_KEY") {
		t.Error("prompt should contain all vault keys")
	}
}

// TestTruncateStr checks boundary conditions for the error-message helper.
func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than ten", 10, "this is lo..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncateStr(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}

// --- Provider constructor tests ---

// TestNewOpenAIProviderDefaults verifies that empty endpoint and model
// arguments are replaced with the documented defaults.
func TestNewOpenAIProviderDefaults(t *testing.T) {
	p := NewOpenAIProvider("", "sk-test", "")
	if p.endpoint != "https://api.openai.com/v1" {
		t.Errorf("endpoint = %q, want default", p.endpoint)
	}
	if p.model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", p.model)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q", p.Name())
	}
	if !p.Available() {
		t.Error("should be available with API key")
	}
}

// TestNewOpenAIProviderCustom checks that a trailing slash is stripped from
// custom endpoints to prevent double-slash in constructed URLs.
func TestNewOpenAIProviderCustom(t *testing.T) {
	p := NewOpenAIProvider("https://openrouter.ai/api/v1/", "key", "claude-3")
	if p.endpoint != "https://openrouter.ai/api/v1" {
		t.Errorf("endpoint = %q, want trailing slash stripped", p.endpoint)
	}
	if p.model != "claude-3" {
		t.Errorf("model = %q", p.model)
	}
}

// TestOpenAIProviderNotAvailable ensures Available returns false when no API
// key is supplied, preventing unnecessary network calls.
func TestOpenAIProviderNotAvailable(t *testing.T) {
	p := NewOpenAIProvider("", "", "")
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

// TestNewAnthropicProviderDefaults verifies that empty endpoint and model
// arguments are replaced with the documented defaults.
func TestNewAnthropicProviderDefaults(t *testing.T) {
	p := NewAnthropicProvider("", "sk-ant-test", "")
	if p.endpoint != "https://api.anthropic.com" {
		t.Errorf("endpoint = %q, want default", p.endpoint)
	}
	if p.model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", p.model)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q", p.Name())
	}
	if !p.Available() {
		t.Error("should be available with API key")
	}
}

// TestAnthropicProviderNotAvailable ensures Available returns false when no
// API key is supplied.
func TestAnthropicProviderNotAvailable(t *testing.T) {
	p := NewAnthropicProvider("", "", "")
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

// --- Bootstrap tests ---

// mockVaultGetter is a test double for VaultKeyGetter backed by a plain map.
type mockVaultGetter struct {
	secrets map[string]string
}

func (m *mockVaultGetter) GetSecret(key string) (*memguard.LockedBuffer, error) {
	val, ok := m.secrets[key]
	if !ok {
		return nil, ErrNoLLMConfigured
	}
	return memguard.NewBufferFromBytes([]byte(val)), nil
}

// TestBootstrapOpenAI verifies that a "openai" provider config produces an
// OpenAIProvider with Available() == true.
func TestBootstrapOpenAI(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{"OPENAI_KEY": "sk-test123"}}
	cfg := Config{
		Provider:    "openai",
		VaultKeyRef: "OPENAI_KEY",
		Model:       "gpt-4o",
	}

	provider, err := Bootstrap(vault, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if provider.Name() != "openai" {
		t.Errorf("Name = %q, want openai", provider.Name())
	}
	if !provider.Available() {
		t.Error("should be available")
	}
}

// TestBootstrapAnthropic verifies that an "anthropic" provider config produces
// an AnthropicProvider.
func TestBootstrapAnthropic(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{"ANTHROPIC_KEY": "sk-ant-test"}}
	cfg := Config{
		Provider:    "anthropic",
		VaultKeyRef: "ANTHROPIC_KEY",
	}

	provider, err := Bootstrap(vault, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if provider.Name() != "anthropic" {
		t.Errorf("Name = %q, want anthropic", provider.Name())
	}
}

// TestBootstrapOpenRouter confirms that "openrouter" maps to the OpenAI-
// compatible provider (OpenRouter speaks the OpenAI wire protocol).
func TestBootstrapOpenRouter(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{"OR_KEY": "sk-or-test"}}
	cfg := Config{
		Provider:    "openrouter",
		Endpoint:    "https://openrouter.ai/api/v1",
		VaultKeyRef: "OR_KEY",
	}

	provider, err := Bootstrap(vault, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if provider.Name() != "openai" {
		t.Errorf("Name = %q, want openai (OpenRouter uses OpenAI-compatible)", provider.Name())
	}
}

// TestBootstrapUnknownProvider confirms that unrecognised provider names fall
// back to the OpenAI-compatible implementation rather than erroring, so users
// can point Vial at any local LLM server.
func TestBootstrapUnknownProvider(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{"KEY": "test"}}
	cfg := Config{
		Provider:    "some-local-llm",
		VaultKeyRef: "KEY",
		Endpoint:    "http://localhost:8080/v1",
	}

	provider, err := Bootstrap(vault, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	// Unknown providers default to OpenAI-compatible
	if provider.Name() != "openai" {
		t.Errorf("Name = %q, want openai", provider.Name())
	}
}

// TestBootstrapNoConfig verifies that a Config with no VaultKeyRef returns
// ErrNoLLMConfigured, allowing callers to disable the LLM tier gracefully.
func TestBootstrapNoConfig(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{}}
	cfg := Config{} // no VaultKeyRef

	_, err := Bootstrap(vault, cfg)
	if err != ErrNoLLMConfigured {
		t.Errorf("expected ErrNoLLMConfigured, got %v", err)
	}
}

// TestBootstrapKeyNotInVault verifies that a missing vault key surfaces as an
// error so the user gets a clear message rather than a silent failure.
func TestBootstrapKeyNotInVault(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{}}
	cfg := Config{
		Provider:    "openai",
		VaultKeyRef: "MISSING_KEY",
	}

	_, err := Bootstrap(vault, cfg)
	if err == nil {
		t.Error("expected error for missing vault key")
	}
}
