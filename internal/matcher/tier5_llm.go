package matcher

import (
	"context"
	"math"
	"time"

	"github.com/cheesejaguar/vial/internal/llm"
)

// LLMMatcher implements Tier 5: LLM-assisted matching.
// It calls an inference API to reason about what a variable likely needs.
type LLMMatcher struct {
	Provider llm.Provider
}

func (m *LLMMatcher) Tier() int    { return 5 }
func (m *LLMMatcher) Name() string { return "llm" }

func (m *LLMMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	if m.Provider == nil || !m.Provider.Available() {
		return nil, nil // gracefully skip if not configured
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := llm.FormatMatchPrompt(requestedKey, vaultKeys)

	resp, err := m.Provider.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: llm.MatchingSystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    200,
		Temperature:  0.0,
	})
	if err != nil {
		return nil, nil // fail open: don't block on LLM errors
	}

	parsed, err := llm.ParseMatchResponse(resp.Content)
	if err != nil {
		return nil, nil // fail open on parse errors
	}

	if parsed.Match == "NO_MATCH" || parsed.Match == "" {
		return nil, nil
	}

	// Verify the match is actually in the vault keys
	found := false
	for _, vk := range vaultKeys {
		if vk == parsed.Match {
			found = true
			break
		}
	}
	if !found {
		return nil, nil // LLM hallucinated a key name
	}

	// Confidence ceiling: LLM matches should never outrank deterministic tiers
	confidence := math.Min(parsed.Confidence, 0.75)

	return []MatchResult{{
		VaultKey:   parsed.Match,
		Confidence: confidence,
		Tier:       5,
		Reason:     "LLM: " + parsed.Reason,
	}}, nil
}
