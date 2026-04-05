// Package matcher implements the 5-tier matching engine that resolves
// environment variable names from a consumer's .env file against key names
// stored in the vault. Tiers run in order from cheapest/most precise to
// most expensive/least precise, and the chain short-circuits as soon as a
// result reaches the high-confidence threshold (>= 0.9).
package matcher

// MatchResult represents a match between a requested key and a vault key.
type MatchResult struct {
	VaultKey   string  // the key in the vault that matched
	Confidence float64 // normalized match strength in the range [0.0, 1.0]
	Tier       int     // which matching tier produced this result (1–5)
	Reason     string  // human-readable explanation shown in CLI output
}

// Matcher is the interface implemented by each tier of the matching engine.
// Each tier encapsulates a single matching strategy and must be safe for
// concurrent use.
type Matcher interface {
	// Match returns zero or more candidates ranked by descending confidence.
	// Implementations must never return an error for "no match" — only for
	// hard failures (I/O, network, etc.). A nil slice means no candidates.
	Match(requestedKey string, vaultKeys []string) ([]MatchResult, error)

	// Tier returns the numeric tier of this matcher (1 = most precise).
	Tier() int

	// Name returns a short identifier used in logs and debug output.
	Name() string
}

// Chain orchestrates multiple Matchers in tier order, implementing an
// early-exit strategy: it stops at the first tier that produces a result
// with confidence >= 0.9, because deterministic tiers at that confidence
// level are considered authoritative and there is no benefit in running
// slower/fuzzier tiers.
type Chain struct {
	matchers []Matcher // ordered slice; lower index = higher priority tier
}

// NewChain creates a new matcher chain containing the supplied matchers in
// the order given. Callers should pass matchers sorted by tier (tier 1 first).
func NewChain(matchers ...Matcher) *Chain {
	return &Chain{matchers: matchers}
}

// Register appends a matcher to the end of the chain. The caller is
// responsible for ordering: later-registered matchers run after earlier ones.
func (c *Chain) Register(m Matcher) {
	c.matchers = append(c.matchers, m)
}

// Resolve finds the best match for requestedKey across all registered tiers.
//
// Strategy:
//   - Run each tier in order.
//   - If a tier returns a result with confidence >= 0.9, return it immediately
//     (high-confidence deterministic match; lower tiers would only be slower
//     and fuzzier).
//   - Otherwise, retain the highest-confidence result seen so far as a fallback
//     and continue to the next tier.
//   - Return the best fallback if no tier reaches the high-confidence threshold.
//   - Return nil if no tier produces any candidate.
func (c *Chain) Resolve(requestedKey string, vaultKeys []string) (*MatchResult, error) {
	var best *MatchResult
	for _, m := range c.matchers {
		results, err := m.Match(requestedKey, vaultKeys)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 && results[0].Confidence >= 0.9 {
			// High-confidence hit — no need to evaluate remaining tiers.
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

// ResolveAll returns every candidate produced by every tier without applying
// the early-exit threshold. This is useful for debugging, introspection UIs,
// and the `brew --dry-run` command that wants to show the user all options.
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
