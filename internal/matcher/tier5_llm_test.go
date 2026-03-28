package matcher

import (
	"context"
	"testing"

	"github.com/cheesejaguar/vial/internal/llm"
)

// mockProvider returns a canned LLM response for testing.
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
