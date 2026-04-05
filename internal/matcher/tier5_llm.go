package matcher

import (
	"context"
	"math"
	"time"

	"github.com/cheesejaguar/vial/internal/llm"
)

// LLMMatcher implements Tier 5: LLM-assisted matching.
//
// When the deterministic tiers (1–4) fail to find a confident match, this tier
// sends the requested key name and the full list of vault key names to an
// inference API and asks it to reason about which vault key is the most likely
// match. This is the most powerful tier but also the most expensive (network
// I/O) and the least reliable (model errors, hallucinations).
//
// Design constraints:
//   - Confidence is capped at 0.75 (via math.Min) so that LLM results can never
//     outrank deterministic tiers whose threshold is 0.9.  This ensures the LLM
//     only ever breaks ties between low-confidence fuzzy results, never overrides
//     a normalized or alias match.
//   - The tier verifies that the key name returned by the model actually exists
//     in vaultKeys before accepting the result. This prevents hallucinated key
//     names from propagating to the caller.
//   - All provider errors and parse errors are treated as "no match" (fail open)
//     rather than surfacing an error to the user. A missing LLM configuration
//     should degrade gracefully, not break the brew pipeline.
//   - Requests are bounded by a 30-second context timeout so a slow or
//     unresponsive provider cannot stall the CLI indefinitely.
type LLMMatcher struct {
	Provider llm.Provider // nil-safe: if nil or unavailable, Match returns nil
}

// Tier returns 5, indicating this is the last resort in the matching chain.
func (m *LLMMatcher) Tier() int { return 5 }

// Name returns the short identifier for this tier.
func (m *LLMMatcher) Name() string { return "llm" }

// Match calls the configured LLM provider to reason about which vault key best
// satisfies requestedKey. It returns nil (no match) in any of these conditions:
//   - Provider is nil or reports itself as unavailable.
//   - The provider returns an error (fail open).
//   - The response cannot be parsed (fail open).
//   - The model returns "NO_MATCH" or an empty match field.
//   - The model returns a key name that does not exist in vaultKeys (hallucination guard).
func (m *LLMMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	if m.Provider == nil || !m.Provider.Available() {
		return nil, nil // gracefully skip if not configured
	}

	// Bound the request so a slow provider cannot stall the CLI.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := llm.FormatMatchPrompt(requestedKey, vaultKeys)

	resp, err := m.Provider.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: llm.MatchingSystemPrompt,
		UserPrompt:   prompt,
		MaxTokens:    200,
		Temperature:  0.0, // deterministic output; creativity would hurt precision here
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

	// Hallucination guard: verify the proposed key actually exists in the vault.
	// LLMs sometimes generate plausible-sounding but non-existent key names;
	// accepting those would silently write wrong values.
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

	// Confidence ceiling: LLM matches should never outrank deterministic tiers.
	// Tiers 1–3 short-circuit at >= 0.9, so capping at 0.75 ensures the LLM
	// is only used as a tiebreaker among low-confidence fuzzy candidates.
	confidence := math.Min(parsed.Confidence, 0.75)

	return []MatchResult{{
		VaultKey:   parsed.Match,
		Confidence: confidence,
		Tier:       5,
		Reason:     "LLM: " + parsed.Reason,
	}}, nil
}
