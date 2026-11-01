package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awnumar/memguard"

	"github.com/cheesejaguar/vial/internal/vault"
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

// --- Provider HTTP tests ---
//
// The LLM APIs are a paid third-party boundary, so they are replaced with a
// local httptest server. The tests assert on the request the provider builds
// and on how it parses each response shape.

// TestOpenAIProviderComplete verifies the request sent to /chat/completions
// (path, auth header, model, system+user messages, default max_tokens) and
// that the first choice is mapped into a CompletionResponse.
func TestOpenAIProviderComplete(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		fmt.Fprint(w, `{"model":"gpt-4o-mini-2024","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"total_tokens":42}}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL+"/v1/", "sk-test", "")
	resp, err := p.Complete(context.Background(), CompletionRequest{
		SystemPrompt: "be terse",
		UserPrompt:   "say hello",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", gotAuth)
	}
	if gotBody.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", gotBody.Model)
	}
	if gotBody.MaxTokens != 200 {
		t.Errorf("max_tokens = %d, want default 200", gotBody.MaxTokens)
	}
	if len(gotBody.Messages) != 2 ||
		gotBody.Messages[0] != (openAIMessage{Role: "system", Content: "be terse"}) ||
		gotBody.Messages[1] != (openAIMessage{Role: "user", Content: "say hello"}) {
		t.Errorf("messages = %+v, want system then user", gotBody.Messages)
	}

	if resp.Content != "hello" || resp.TokensUsed != 42 || resp.Model != "gpt-4o-mini-2024" || resp.FinishReason != "stop" {
		t.Errorf("response = %+v", resp)
	}
}

// TestOpenAIProviderOmitsEmptySystemPrompt verifies that no system message is
// sent when SystemPrompt is empty.
func TestOpenAIProviderOmitsEmptySystemPrompt(t *testing.T) {
	var gotBody openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "k", "m")
	if _, err := p.Complete(context.Background(), CompletionRequest{UserPrompt: "u", MaxTokens: 50}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Errorf("messages = %+v, want only the user message", gotBody.Messages)
	}
	if gotBody.MaxTokens != 50 {
		t.Errorf("max_tokens = %d, want 50", gotBody.MaxTokens)
	}
}

// TestOpenAIProviderErrors verifies that an in-body API error and an empty
// choices list are both surfaced as ErrProviderError.
func TestOpenAIProviderErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"api error in body", `{"error":{"message":"invalid api key"}}`, "invalid api key"},
		{"no choices", `{"choices":[]}`, "no choices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			_, err := NewOpenAIProvider(srv.URL, "k", "").Complete(context.Background(), CompletionRequest{UserPrompt: "u"})
			if !errors.Is(err, ErrProviderError) {
				t.Fatalf("err = %v, want ErrProviderError", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestOpenAIProviderOpenRouterHeaders verifies the attribution headers are
// added only when the endpoint is OpenRouter.
func TestOpenAIProviderOpenRouterHeaders(t *testing.T) {
	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("X-Title")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	if _, err := NewOpenAIProvider(srv.URL, "k", "").Complete(context.Background(), CompletionRequest{UserPrompt: "u"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotTitle != "" {
		t.Errorf("X-Title = %q on a non-OpenRouter endpoint, want none", gotTitle)
	}

	// Route an "openrouter" URL to the test server via a custom transport.
	p := NewOpenAIProvider("http://openrouter.test/api/v1", "k", "")
	p.client = &http.Client{Transport: rewriteHostTransport{target: srv.URL}}
	if _, err := p.Complete(context.Background(), CompletionRequest{UserPrompt: "u"}); err != nil {
		t.Fatalf("Complete via openrouter: %v", err)
	}
	if gotTitle != "Vial Secret Vault" {
		t.Errorf("X-Title = %q, want Vial Secret Vault", gotTitle)
	}
}

// rewriteHostTransport sends every request to target, keeping the path. It
// lets a test use an endpoint URL whose host triggers provider-specific logic.
type rewriteHostTransport struct{ target string }

func (rt rewriteHostTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u, err := url.Parse(rt.target)
	if err != nil {
		return nil, err
	}
	r = r.Clone(r.Context())
	r.URL.Scheme = u.Scheme
	r.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(r)
}

// TestAnthropicProviderComplete verifies the request sent to /v1/messages
// (headers, top-level system prompt, default max_tokens) and that text blocks
// are concatenated while non-text blocks are ignored.
func TestAnthropicProviderComplete(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		fmt.Fprint(w, `{"model":"claude-x","stop_reason":"end_turn","content":[{"type":"text","text":"foo"},{"type":"tool_use"},{"type":"text","text":"bar"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(srv.URL+"/", "sk-ant", "")
	resp, err := p.Complete(context.Background(), CompletionRequest{SystemPrompt: "sys", UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "sk-ant" {
		t.Errorf("x-api-key = %q, want sk-ant", gotKey)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want 2023-06-01", gotVersion)
	}
	if gotBody.Model != "claude-sonnet-4-6" || gotBody.MaxTokens != 200 || gotBody.System != "sys" {
		t.Errorf("request body = %+v", gotBody)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0] != (anthropicMessage{Role: "user", Content: "hi"}) {
		t.Errorf("messages = %+v, want a single user message", gotBody.Messages)
	}

	if resp.Content != "foobar" {
		t.Errorf("Content = %q, want foobar (text blocks concatenated)", resp.Content)
	}
	if resp.TokensUsed != 15 || resp.Model != "claude-x" || resp.FinishReason != "end_turn" {
		t.Errorf("response = %+v", resp)
	}
}

// TestAnthropicProviderErrors verifies that an in-body API error and an empty
// content list are both surfaced as ErrProviderError.
func TestAnthropicProviderErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"api error in body", `{"error":{"type":"authentication_error","message":"bad key"}}`, "bad key"},
		{"no content", `{"content":[]}`, "no content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			_, err := NewAnthropicProvider(srv.URL, "k", "").Complete(context.Background(), CompletionRequest{UserPrompt: "u"})
			if !errors.Is(err, ErrProviderError) {
				t.Fatalf("err = %v, want ErrProviderError", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// --- Bootstrap tests ---
//
// Bootstrap reads the API key from a real (test-KDF) vault rather than a
// hand-written stand-in, so the VaultKeyGetter contract is checked against
// the actual *vault.VaultManager.

// newTestVault returns an unlocked vault containing the given secrets.
func newTestVault(t *testing.T, secrets map[string]string) *vault.VaultManager {
	t.Helper()
	vm := vault.NewVaultManager(filepath.Join(t.TempDir(), "vault.json"))
	vm.SetKDFParams(vault.TestKDFParams())

	pw := memguard.NewBufferFromBytes([]byte("test-password-12chars"))
	defer pw.Destroy()
	if err := vm.Init(pw); err != nil {
		t.Fatalf("Init vault: %v", err)
	}
	for k, v := range secrets {
		buf := memguard.NewBufferFromBytes([]byte(v))
		if err := vm.SetSecret(k, buf); err != nil {
			t.Fatalf("SetSecret %s: %v", k, err)
		}
		buf.Destroy()
	}
	t.Cleanup(vm.Lock)
	return vm
}

// TestBootstrapOpenAI verifies that an "openai" provider config produces an
// OpenAIProvider carrying the API key read from the vault and the configured
// model.
func TestBootstrapOpenAI(t *testing.T) {
	vm := newTestVault(t, map[string]string{"OPENAI_KEY": "sk-test123"})
	cfg := Config{
		Provider:    "openai",
		VaultKeyRef: "OPENAI_KEY",
		Model:       "gpt-4o",
	}

	provider, err := Bootstrap(vm, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	p, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *OpenAIProvider", provider)
	}
	if p.apiKey != "sk-test123" {
		t.Errorf("apiKey = %q, want the value stored in the vault", p.apiKey)
	}
	if p.model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", p.model)
	}
	if p.endpoint != "https://api.openai.com/v1" {
		t.Errorf("endpoint = %q, want default", p.endpoint)
	}
}

// TestBootstrapAnthropic verifies that an "anthropic" provider config produces
// an AnthropicProvider with the vault key and default model.
func TestBootstrapAnthropic(t *testing.T) {
	vm := newTestVault(t, map[string]string{"ANTHROPIC_KEY": "sk-ant-test"})
	cfg := Config{
		Provider:    "anthropic",
		VaultKeyRef: "ANTHROPIC_KEY",
	}

	provider, err := Bootstrap(vm, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	p, ok := provider.(*AnthropicProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *AnthropicProvider", provider)
	}
	if p.apiKey != "sk-ant-test" {
		t.Errorf("apiKey = %q, want the value stored in the vault", p.apiKey)
	}
	if p.model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want default claude-sonnet-4-6", p.model)
	}
}

// TestBootstrapOpenRouter confirms that "openrouter" maps to the OpenAI-
// compatible provider (OpenRouter speaks the OpenAI wire protocol) and keeps
// the configured endpoint.
func TestBootstrapOpenRouter(t *testing.T) {
	vm := newTestVault(t, map[string]string{"OR_KEY": "sk-or-test"})
	cfg := Config{
		Provider:    "openrouter",
		Endpoint:    "https://openrouter.ai/api/v1",
		VaultKeyRef: "OR_KEY",
	}

	provider, err := Bootstrap(vm, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	p, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *OpenAIProvider (OpenRouter is OpenAI-compatible)", provider)
	}
	if p.endpoint != "https://openrouter.ai/api/v1" || p.apiKey != "sk-or-test" {
		t.Errorf("endpoint=%q apiKey=%q", p.endpoint, p.apiKey)
	}
}

// TestBootstrapUnknownProvider confirms that unrecognised provider names fall
// back to the OpenAI-compatible implementation rather than erroring, so users
// can point Vial at any local LLM server.
func TestBootstrapUnknownProvider(t *testing.T) {
	vm := newTestVault(t, map[string]string{"KEY": "test"})
	cfg := Config{
		Provider:    "some-local-llm",
		VaultKeyRef: "KEY",
		Endpoint:    "http://localhost:8080/v1",
	}

	provider, err := Bootstrap(vm, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	p, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *OpenAIProvider", provider)
	}
	if p.endpoint != "http://localhost:8080/v1" {
		t.Errorf("endpoint = %q, want the configured local endpoint", p.endpoint)
	}
}

// TestBootstrapNoConfig verifies that a Config with no VaultKeyRef returns
// ErrNoLLMConfigured, allowing callers to disable the LLM tier gracefully.
func TestBootstrapNoConfig(t *testing.T) {
	vm := newTestVault(t, nil)
	cfg := Config{} // no VaultKeyRef

	_, err := Bootstrap(vm, cfg)
	if err != ErrNoLLMConfigured {
		t.Errorf("expected ErrNoLLMConfigured, got %v", err)
	}
}

// TestBootstrapKeyNotInVault verifies that a missing vault key surfaces as an
// error that names the key and wraps the vault's not-found error.
func TestBootstrapKeyNotInVault(t *testing.T) {
	vm := newTestVault(t, nil)
	cfg := Config{
		Provider:    "openai",
		VaultKeyRef: "MISSING_KEY",
	}

	_, err := Bootstrap(vm, cfg)
	if !errors.Is(err, vault.ErrSecretNotFound) {
		t.Fatalf("err = %v, want it to wrap vault.ErrSecretNotFound", err)
	}
	if !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Errorf("err = %q, want it to name the missing key", err)
	}
}
