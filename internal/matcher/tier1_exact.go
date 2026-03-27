package matcher

// ExactMatcher implements Tier 1: exact case-sensitive matching.
type ExactMatcher struct{}

func (m *ExactMatcher) Tier() int    { return 1 }
func (m *ExactMatcher) Name() string { return "exact" }

// Match returns a result if the requested key exactly matches a vault key.
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
