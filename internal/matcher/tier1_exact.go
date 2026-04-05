package matcher

// ExactMatcher implements Tier 1: exact case-sensitive string matching.
//
// This is the cheapest and most authoritative tier. A character-perfect match
// means there is no ambiguity, so confidence is always 1.0. The chain will
// short-circuit here and never invoke lower tiers.
type ExactMatcher struct{}

// Tier returns 1, indicating this is the highest-priority matching tier.
func (m *ExactMatcher) Tier() int { return 1 }

// Name returns the short identifier for this tier.
func (m *ExactMatcher) Name() string { return "exact" }

// Match returns a single result with confidence 1.0 if requestedKey is found
// verbatim in vaultKeys, or nil if no exact match exists. Case differences
// such as "openai_api_key" vs "OPENAI_API_KEY" are handled by Tier 2.
func (m *ExactMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	for _, vk := range vaultKeys {
		if vk == requestedKey {
			return []MatchResult{{
				VaultKey:   vk,
				Confidence: 1.0,
				Tier:       1,
				Reason:     "exact match",
			}}, nil
		}
	}
	return nil, nil
}
