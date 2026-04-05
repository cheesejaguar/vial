package matcher

import (
	"strings"

	"github.com/cheesejaguar/vial/internal/alias"
)

// NormalizeMatcher implements Tier 2: case-insensitive matching with
// framework-prefix stripping.
//
// Many frameworks require environment variables to carry a mandatory prefix
// that has no semantic meaning for the secret itself:
//   - Next.js public variables: NEXT_PUBLIC_
//   - Vite variables: VITE_
//   - Create React App variables: REACT_APP_
//
// This tier normalizes both the requested key and each vault key via
// alias.Normalize (uppercase + prefix strip) and compares the results.
// Confidence is slightly lower than an exact match because prefix stripping
// introduces a small degree of inference:
//   - Case-only difference → 0.90 (near-certain, just a convention mismatch)
//   - Prefix was stripped → 0.85 (still highly likely but one inference step removed)
type NormalizeMatcher struct{}

// Tier returns 2, indicating this runs after exact matching but before alias lookup.
func (m *NormalizeMatcher) Tier() int { return 2 }

// Name returns the short identifier for this tier.
func (m *NormalizeMatcher) Name() string { return "normalize" }

// Match normalizes requestedKey and each vault key, returning a result whenever
// the two normalized forms are equal. All matching vault keys are returned so
// that the chain's best-result logic can pick among them if more than one
// prefix-stripped key collapses to the same canonical form.
func (m *NormalizeMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	normalizedReq := alias.Normalize(requestedKey)

	var results []MatchResult
	for _, vk := range vaultKeys {
		normalizedVK := alias.Normalize(vk)
		if normalizedReq == normalizedVK {
			confidence := 0.90
			reason := "case-insensitive match"

			// Detect whether normalization did more than just uppercase the string.
			// If the uppercase form differs from the normalized form for either side,
			// a framework prefix was removed, which is a slightly weaker inference.
			reqUpperOnly := strings.ToUpper(requestedKey)
			vkUpperOnly := strings.ToUpper(vk)
			if normalizedReq != reqUpperOnly || normalizedVK != vkUpperOnly {
				// Normalization changed more than just case — a prefix was stripped
				reason = "prefix-stripped match"
				confidence = 0.85
			}
			results = append(results, MatchResult{
				VaultKey:   vk,
				Confidence: confidence,
				Tier:       2,
				Reason:     reason,
			})
		}
	}

	return results, nil
}
