package matcher

import (
	"strings"
)

// CommentMatcher implements Tier 4: comment-informed matching.
// It extracts keywords from .env.example comments and matches them
// against vault key names using keyword overlap scoring.
type CommentMatcher struct{}

func (m *CommentMatcher) Tier() int    { return 4 }
func (m *CommentMatcher) Name() string { return "comment" }

// MatchWithComment attempts to match using the comment associated with a key.
// This is called with the comment text from the .env.example entry.
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

		if score > bestScore && score >= 0.3 {
			bestScore = score
			best = &MatchResult{
				VaultKey:   vk,
				Confidence: clamp(score*0.85, 0, 0.85), // cap at 0.85
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

// Match implements the Matcher interface but requires comment context.
// Without comments, it returns nil. Use MatchWithComment for full functionality.
func (m *CommentMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	return nil, nil
}

// serviceNames maps common service keywords to canonical names found in vault keys.
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

// purposeWords maps purpose keywords to common key suffixes.
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

// extractKeywords pulls meaningful words from a comment string.
func extractKeywords(comment string) []string {
	comment = strings.ToLower(strings.TrimSpace(comment))

	// Remove common noise words
	noise := map[string]bool{
		"the": true, "a": true, "an": true, "your": true, "my": true,
		"for": true, "to": true, "of": true, "in": true, "is": true,
		"this": true, "that": true, "it": true, "from": true, "with": true,
		"and": true, "or": true, "not": true, "set": true, "here": true,
		"get": true, "put": true, "see": true, "use": true, "used": true,
		"e.g": true, "eg": true, "i.e": true, "etc": true,
	}

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

// splitKeyName splits a vault key name into words.
// e.g., "OPENAI_API_KEY" → ["OPENAI", "API", "KEY"]
func splitKeyName(key string) []string {
	return strings.Split(strings.ToUpper(key), "_")
}

// keywordOverlap computes a normalized overlap score between comment keywords
// and vault key name parts, considering service name and purpose mappings.
func keywordOverlap(commentWords, keyParts []string) float64 {
	if len(commentWords) == 0 || len(keyParts) == 0 {
		return 0
	}

	keyPartSet := make(map[string]bool)
	for _, p := range keyParts {
		keyPartSet[strings.ToUpper(p)] = true
	}

	matches := 0
	total := len(commentWords)

	for _, cw := range commentWords {
		cwUpper := strings.ToUpper(cw)

		// Direct word match
		if keyPartSet[cwUpper] {
			matches++
			continue
		}

		// Service name expansion
		if expansions, ok := serviceNames[cw]; ok {
			for _, exp := range expansions {
				if keyPartSet[exp] {
					matches++
					break
				}
			}
			continue
		}

		// Purpose word expansion
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

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
