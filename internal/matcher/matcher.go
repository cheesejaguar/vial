package matcher

// MatchResult represents a match between a requested key and a vault key.
type MatchResult struct {
	VaultKey   string  // The key in the vault that matched
	Confidence float64 // 0.0 to 1.0
	Tier       int     // Which matching tier produced this result (1-5)
	Reason     string  // Human-readable explanation
}

// Matcher is the interface for a single matching strategy.
type Matcher interface {
	Match(requestedKey string, vaultKeys []string) ([]MatchResult, error)
	Tier() int
	Name() string
}

// Chain orchestrates multiple Matchers in tier order.
type Chain struct {
	matchers []Matcher
}

// NewChain creates a new matcher chain.
func NewChain(matchers ...Matcher) *Chain {
	return &Chain{matchers: matchers}
}

// Register adds a matcher to the chain.
func (c *Chain) Register(m Matcher) {
	c.matchers = append(c.matchers, m)
}

// Resolve finds the best match across all tiers, stopping at the first
// tier that produces a high-confidence result (>= 0.9). Lower-confidence
// results are accumulated as fallback, and the highest-confidence match wins.
func (c *Chain) Resolve(requestedKey string, vaultKeys []string) (*MatchResult, error) {
	var best *MatchResult
	for _, m := range c.matchers {
		results, err := m.Match(requestedKey, vaultKeys)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 && results[0].Confidence >= 0.9 {
			return &results[0], nil
		}
		// Keep the highest-confidence result as fallback
		if len(results) > 0 && (best == nil || results[0].Confidence > best.Confidence) {
			r := results[0]
			best = &r
		}
	}
	return best, nil
}

// ResolveAll returns all candidates across all tiers.
func (c *Chain) ResolveAll(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	var all []MatchResult
	for _, m := range c.matchers {
		results, err := m.Match(requestedKey, vaultKeys)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return all, nil
}
