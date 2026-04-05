package matcher

import (
	"strings"
)

// CommentMatcher implements Tier 4: comment-informed matching.
//
// When a .env.example file contains human-readable comments above or beside a
// key (e.g. "# Your Stripe secret key for payment processing"), those words
// often reveal the intent behind a generic or abbreviated key name. This tier
// extracts meaningful keywords from the comment and measures their overlap with
// the words that make up each vault key, using two vocabulary tables:
//   - serviceNames: maps lowercase service words (e.g. "stripe") to their
//     canonical uppercase forms used in vault keys (e.g. "STRIPE").
//   - purposeWords: maps purpose-indicating words (e.g. "key", "token") to the
//     key-name suffixes they commonly correspond to (e.g. "KEY", "API_KEY").
//
// Confidence is capped at 0.85 so that comment-based matches never outrank the
// deterministic tiers 1–3, which have stronger guarantees. The full-confidence
// ceiling leaves room for tiers 1 and 2 to remain authoritative at >= 0.9.
//
// Note: the Matcher interface's Match method returns nil because comment text
// is not available in that signature. Callers that have comment context must
// use MatchWithComment directly.
type CommentMatcher struct{}

// Tier returns 4, indicating this runs after alias matching but before LLM inference.
func (m *CommentMatcher) Tier() int { return 4 }

// Name returns the short identifier for this tier.
func (m *CommentMatcher) Name() string { return "comment" }

// MatchWithComment attempts to match requestedKey to a vault key using comment
// text extracted from a .env.example entry. The comment is tokenized into
// keywords, noise words are removed, and the remaining tokens are scored
// against each vault key's name parts via keywordOverlap.
//
// A minimum raw overlap score of 0.3 is required before a result is emitted,
// which filters out accidental single-word matches on common terms. Confidence
// is scaled from the raw overlap score and capped at 0.85.
func (m *CommentMatcher) MatchWithComment(requestedKey, comment string, vaultKeys []string) ([]MatchResult, error) {
	if comment == "" {
		return nil, nil
	}

	commentWords := extractKeywords(comment)
	if len(commentWords) == 0 {
		return nil, nil
	}

	var best *MatchResult
	bestScore := 0.0

	for _, vk := range vaultKeys {
		vkWords := splitKeyName(vk)
		score := keywordOverlap(commentWords, vkWords)

		// Require a minimum overlap of 0.3 to avoid spurious matches on
		// incidental common words that appear in both the comment and the key.
		if score > bestScore && score >= 0.3 {
			bestScore = score
			best = &MatchResult{
				VaultKey: vk,
				// Scale the raw [0,1] overlap score into the [0, 0.85] band.
				// Multiplying by 0.85 before clamping ensures that even a
				// perfect overlap (score = 1.0) stays below the 0.9 threshold
				// that would let it short-circuit the chain.
				Confidence: clamp(score*0.85, 0, 0.85),
				Tier:       4,
				Reason:     "comment match: \"" + truncate(comment, 40) + "\"",
			}
		}
	}

	if best != nil {
		return []MatchResult{*best}, nil
	}
	return nil, nil
}

// Match implements the Matcher interface. Because the Matcher.Match signature
// does not carry comment text, this always returns nil. Chain users that want
// Tier 4 to participate should call MatchWithComment directly from the brew
// pipeline after parsing the .env.example file.
func (m *CommentMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	return nil, nil
}

// serviceNames maps lowercase service keywords found in comments to the
// uppercase fragments used inside vault key names. Multiple canonical forms
// are listed where a service has common abbreviations (e.g. Google Cloud
// Platform is referenced both as "GOOGLE" and "GCP" in vault keys).
var serviceNames = map[string][]string{
	"openai":     {"OPENAI"},
	"stripe":     {"STRIPE"},
	"supabase":   {"SUPABASE"},
	"firebase":   {"FIREBASE"},
	"mapbox":     {"MAPBOX"},
	"twilio":     {"TWILIO"},
	"sendgrid":   {"SENDGRID"},
	"aws":        {"AWS"},
	"azure":      {"AZURE"},
	"google":     {"GOOGLE", "GCP"},
	"github":     {"GITHUB", "GH"},
	"gitlab":     {"GITLAB"},
	"cloudflare": {"CLOUDFLARE", "CF"},
	"redis":      {"REDIS"},
	"postgres":   {"POSTGRES", "PG", "DATABASE", "DB"},
	"postgresql": {"POSTGRES", "PG", "DATABASE", "DB"},
	"mysql":      {"MYSQL", "DATABASE", "DB"},
	"mongodb":    {"MONGODB", "MONGO"},
	"anthropic":  {"ANTHROPIC", "CLAUDE"},
	"claude":     {"ANTHROPIC", "CLAUDE"},
	"sentry":     {"SENTRY"},
	"datadog":    {"DATADOG", "DD"},
	"slack":      {"SLACK"},
	"discord":    {"DISCORD"},
	"resend":     {"RESEND"},
	"postmark":   {"POSTMARK"},
	"algolia":    {"ALGOLIA"},
	"pinecone":   {"PINECONE"},
	"vercel":     {"VERCEL"},
	"netlify":    {"NETLIFY"},
	"heroku":     {"HEROKU"},
}

// purposeWords maps generic purpose-indicating words in comments to the key-name
// segments they typically correspond to. Multi-word expansions (e.g. "API_KEY")
// are split on "_" before comparison, so each part is checked independently.
var purposeWords = map[string][]string{
	"key":        {"KEY", "API_KEY", "SECRET_KEY"},
	"secret":     {"SECRET", "SECRET_KEY"},
	"token":      {"TOKEN", "ACCESS_TOKEN"},
	"password":   {"PASSWORD", "PASS", "PW"},
	"url":        {"URL", "URI", "ENDPOINT"},
	"host":       {"HOST", "HOSTNAME"},
	"port":       {"PORT"},
	"database":   {"DATABASE", "DB"},
	"connection": {"URL", "URI", "CONNECTION_STRING"},
}

// extractKeywords tokenizes a comment string into lowercase alphanumeric words,
// removing single-character tokens and a curated stop-word list. Stop words are
// common English words that appear frequently in comments but carry no signal
// about which vault key to choose (e.g. "the", "your", "for").
func extractKeywords(comment string) []string {
	comment = strings.ToLower(strings.TrimSpace(comment))

	// Remove common noise words that appear in comments but carry no signal
	// about which vault key is the right match.
	noise := map[string]bool{
		"the": true, "a": true, "an": true, "your": true, "my": true,
		"for": true, "to": true, "of": true, "in": true, "is": true,
		"this": true, "that": true, "it": true, "from": true, "with": true,
		"and": true, "or": true, "not": true, "set": true, "here": true,
		"get": true, "put": true, "see": true, "use": true, "used": true,
		"e.g": true, "eg": true, "i.e": true, "etc": true,
	}

	// Split on anything that is not a lowercase letter, digit, or underscore.
	// Underscores are kept so that multi-word tokens like "api_key" survive
	// intact if a user writes them that way in a comment.
	words := strings.FieldsFunc(comment, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})

	var keywords []string
	for _, w := range words {
		if len(w) > 1 && !noise[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// splitKeyName splits a vault key name into its constituent word parts by
// splitting on underscores and uppercasing the result. For example:
// "OPENAI_API_KEY" → ["OPENAI", "API", "KEY"].
func splitKeyName(key string) []string {
	return strings.Split(strings.ToUpper(key), "_")
}

// keywordOverlap computes a normalized overlap score between a set of comment
// keywords and the parts of a vault key name. The score is the fraction of
// comment keywords that match at least one part of the vault key, either
// directly or via the serviceNames and purposeWords vocabulary expansions.
//
// The denominator is always len(commentWords), which means a comment with many
// words requires proportionally more matches to score highly — naturally
// penalizing vague one-word comments and rewarding specific multi-word ones.
func keywordOverlap(commentWords, keyParts []string) float64 {
	if len(commentWords) == 0 || len(keyParts) == 0 {
		return 0
	}

	// Build an uppercase set for O(1) lookup of key parts.
	keyPartSet := make(map[string]bool)
	for _, p := range keyParts {
		keyPartSet[strings.ToUpper(p)] = true
	}

	matches := 0
	total := len(commentWords)

	for _, cw := range commentWords {
		cwUpper := strings.ToUpper(cw)

		// Direct word match: the comment word exactly equals a key part
		// (e.g. comment "OPENAI" matches key part "OPENAI").
		if keyPartSet[cwUpper] {
			matches++
			continue
		}

		// Service name expansion: look up lowercase comment word in the
		// vocabulary table and check if any of its canonical forms appear
		// in the key parts (e.g. "google" → {"GOOGLE", "GCP"}).
		if expansions, ok := serviceNames[cw]; ok {
			for _, exp := range expansions {
				if keyPartSet[exp] {
					matches++
					break
				}
			}
			continue
		}

		// Purpose word expansion: look up the comment word and check each
		// expansion's individual underscore-split parts against the key part
		// set (e.g. "key" → {"KEY", "API_KEY"} → check "KEY" and "API").
		if expansions, ok := purposeWords[cw]; ok {
			for _, exp := range expansions {
				parts := strings.Split(exp, "_")
				for _, part := range parts {
					if keyPartSet[part] {
						matches++
						break
					}
				}
			}
		}
	}

	return float64(matches) / float64(total)
}

// clamp returns v clamped to the closed interval [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// truncate returns s unchanged when len(s) <= n, otherwise returns the first n
// bytes followed by "..." for use in human-readable Reason strings.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
