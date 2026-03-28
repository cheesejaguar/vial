package matcher

import (
	"strings"

	"github.com/cheesejaguar/vial/internal/alias"
)

// NormalizeMatcher implements Tier 2: case-insensitive matching with
// framework prefix stripping (NEXT_PUBLIC_, VITE_, REACT_APP_, etc).
type NormalizeMatcher struct{}

func (m *NormalizeMatcher) Tier() int    { return 2 }
func (m *NormalizeMatcher) Name() string { return "normalize" }

func (m *NormalizeMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	normalizedReq := alias.Normalize(requestedKey)

	var results []MatchResult
	for _, vk := range vaultKeys {
		normalizedVK := alias.Normalize(vk)
		if normalizedReq == normalizedVK {
			confidence := 0.90
			reason := "case-insensitive match"
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
