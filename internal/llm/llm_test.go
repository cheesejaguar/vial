package llm

import (
	"strings"
	"testing"

	"github.com/awnumar/memguard"
)

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
			name:      "markdown code fence",
			raw:       "```json\n{\"match\": \"STRIPE_KEY\", \"confidence\": 0.8, \"reason\": \"stripe key\"}\n```",
			wantMatch: "STRIPE_KEY",
			wantConf:  0.8,
		},
		{
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

func TestNewOpenAIProviderCustom(t *testing.T) {
	p := NewOpenAIProvider("https://openrouter.ai/api/v1/", "key", "claude-3")
	if p.endpoint != "https://openrouter.ai/api/v1" {
		t.Errorf("endpoint = %q, want trailing slash stripped", p.endpoint)
	}
	if p.model != "claude-3" {
		t.Errorf("model = %q", p.model)
	}
}

func TestOpenAIProviderNotAvailable(t *testing.T) {
	p := NewOpenAIProvider("", "", "")
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

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

func TestAnthropicProviderNotAvailable(t *testing.T) {
	p := NewAnthropicProvider("", "", "")
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

// --- Bootstrap tests ---

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

func TestBootstrapNoConfig(t *testing.T) {
	vault := &mockVaultGetter{secrets: map[string]string{}}
	cfg := Config{} // no VaultKeyRef

	_, err := Bootstrap(vault, cfg)
	if err != ErrNoLLMConfigured {
		t.Errorf("expected ErrNoLLMConfigured, got %v", err)
	}
}

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
