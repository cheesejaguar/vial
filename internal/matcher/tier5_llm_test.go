package matcher

import (
	"context"
	"testing"

	"github.com/cheesejaguar/vial/internal/llm"
)

// mockProvider returns a canned LLM response for testing without making real
// network calls. The called field lets tests assert whether the provider was
// invoked at all.
type mockProvider struct {
	response string
	err      error
	called   bool
}

func (m *mockProvider) Complete(_ context.Context, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &llm.CompletionResponse{Content: m.response}, nil
}

func (m *mockProvider) Name() string    { return "mock" }
func (m *mockProvider) Available() bool { return true }

// TestLLMMatcherSuccess verifies that a valid LLM response is accepted and
// that the confidence is capped at 0.75 even when the model returns 0.9.
func TestLLMMatcherSuccess(t *testing.T) {
	mock := &mockProvider{
		response: `{"match": "OPENAI_API_KEY", "confidence": 0.9, "reason": "common variant"}`,
	}
	m := &LLMMatcher{Provider: mock}

	results, err := m.Match("OPENAI_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].VaultKey != "OPENAI_API_KEY" {
		t.Errorf("VaultKey = %q, want OPENAI_API_KEY", results[0].VaultKey)
	}
	// Confidence should be capped at 0.75
	if results[0].Confidence != 0.75 {
		t.Errorf("Confidence = %f, want 0.75 (capped)", results[0].Confidence)
	}
	if results[0].Tier != 5 {
		t.Errorf("Tier = %d, want 5", results[0].Tier)
	}
}

// TestLLMMatcherNoMatch verifies that a "NO_MATCH" response from the model
// results in nil being returned rather than an error.
func TestLLMMatcherNoMatch(t *testing.T) {
	mock := &mockProvider{
		response: `{"match": "NO_MATCH", "confidence": 0.0, "reason": "no suitable match"}`,
	}
	m := &LLMMatcher{Provider: mock}

	results, err := m.Match("RANDOM_KEY", []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

// TestLLMMatcherMalformedResponse verifies fail-open behavior: a non-JSON
// response returns nil rather than propagating a parse error.
func TestLLMMatcherMalformedResponse(t *testing.T) {
	mock := &mockProvider{response: "not json at all"}
	m := &LLMMatcher{Provider: mock}

	results, err := m.Match("KEY", []string{"OTHER_KEY"})
	if err != nil {
		t.Fatalf("should fail open, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for malformed response, got %v", results)
	}
}

// TestLLMMatcherProviderNil verifies that a nil provider is handled gracefully
// without panicking.
func TestLLMMatcherProviderNil(t *testing.T) {
	m := &LLMMatcher{Provider: nil}

	results, err := m.Match("KEY", []string{"OTHER_KEY"})
	if err != nil {
		t.Fatalf("should return nil, got error: %v", err)
	}
	if results != nil {
		t.Error("expected nil for nil provider")
	}
}

// TestLLMMatcherHallucinatedKey verifies that a key name returned by the model
// that does not exist in the vault is rejected, preventing hallucinated keys
// from propagating to the caller.
func TestLLMMatcherHallucinatedKey(t *testing.T) {
	mock := &mockProvider{
		response: `{"match": "NONEXISTENT_KEY", "confidence": 0.9, "reason": "hallucinated"}`,
	}
	m := &LLMMatcher{Provider: mock}

	results, err := m.Match("KEY", []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if results != nil {
		t.Error("should reject hallucinated key names")
	}
}

// TestLLMMatcherProviderError verifies fail-open behavior: a provider-level
// error returns nil rather than surfacing the error to the caller.
func TestLLMMatcherProviderError(t *testing.T) {
	mock := &mockProvider{err: llm.ErrProviderError}
	m := &LLMMatcher{Provider: mock}

	results, err := m.Match("KEY", []string{"OTHER_KEY"})
	if err != nil {
		t.Fatalf("should fail open, got error: %v", err)
	}
	if results != nil {
		t.Error("expected nil on provider error")
	}
}

// TestChainSkipsLLMWhenExactMatches verifies the chain's early-exit behavior:
// the LLM provider must not be called when a high-confidence exact match
// already satisfies the threshold.
func TestChainSkipsLLMWhenExactMatches(t *testing.T) {
	mock := &mockProvider{}
	chain := NewChain(
		&ExactMatcher{},
		&LLMMatcher{Provider: mock},
	)

	result, err := chain.Resolve("EXACT_KEY", []string{"EXACT_KEY"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Tier != 1 {
		t.Errorf("Tier = %d, want 1", result.Tier)
	}
	if mock.called {
		t.Error("LLM should not have been called when exact match exists")
	}
}
