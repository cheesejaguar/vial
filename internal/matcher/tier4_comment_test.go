package matcher

import (
	"testing"
)

// TestCommentMatcherBasic verifies that service-name vocabulary expansion
// correctly maps comment keywords to vault keys across different service types.
func TestCommentMatcherBasic(t *testing.T) {
	m := &CommentMatcher{}
	vault := []string{"STRIPE_SECRET_KEY", "STRIPE_PUBLISHABLE_KEY", "OPENAI_API_KEY", "DATABASE_URL"}

	tests := []struct {
		name    string
		key     string
		comment string
		wantKey string
	}{
		{"stripe secret", "PAYMENT_KEY", "Your Stripe secret key for payment processing", "STRIPE_SECRET_KEY"},
		{"openai key", "AI_KEY", "OpenAI API key for AI features", "OPENAI_API_KEY"},
		{"database", "DB_URL", "The database connection string PostgreSQL", "DATABASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := m.MatchWithComment(tt.key, tt.comment, vault)
			if err != nil {
				t.Fatalf("MatchWithComment: %v", err)
			}
			if len(results) == 0 {
				t.Fatalf("expected a match for comment %q", tt.comment)
			}
			if results[0].VaultKey != tt.wantKey {
				t.Errorf("VaultKey = %q, want %q", results[0].VaultKey, tt.wantKey)
			}
			if results[0].Tier != 4 {
				t.Errorf("Tier = %d, want 4", results[0].Tier)
			}
			if results[0].Confidence > 0.85 {
				t.Errorf("Confidence = %f, should be capped at 0.85", results[0].Confidence)
			}
		})
	}
}

// TestCommentMatcherEmptyComment verifies that an empty comment produces no results
// rather than a spurious match or an error.
func TestCommentMatcherEmptyComment(t *testing.T) {
	m := &CommentMatcher{}
	results, _ := m.MatchWithComment("KEY", "", []string{"OPENAI_API_KEY"})
	if results != nil {
		t.Error("expected nil for empty comment")
	}
}

// TestCommentMatcherNoMatch verifies that comments with no vocabulary overlap
// do not produce results, guarding against false positives on generic text.
func TestCommentMatcherNoMatch(t *testing.T) {
	m := &CommentMatcher{}
	results, _ := m.MatchWithComment("KEY", "completely unrelated comment about nothing", []string{"OPENAI_API_KEY"})
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

// TestExtractKeywords verifies that extractKeywords correctly tokenizes comment
// text and removes stop words while preserving meaningful terms.
func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		comment string
		wantAny []string
	}{
		{"Your Stripe secret key", []string{"stripe", "secret", "key"}},
		{"The OpenAI API key for AI features", []string{"openai", "api", "key", "ai", "features"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := extractKeywords(tt.comment)
		gotSet := make(map[string]bool)
		for _, g := range got {
			gotSet[g] = true
		}
		for _, want := range tt.wantAny {
			if !gotSet[want] {
				t.Errorf("extractKeywords(%q) missing %q (got %v)", tt.comment, want, got)
			}
		}
	}
}

// TestKeywordOverlap verifies the overlap scoring function across direct matches,
// service-name expansions, and cases with no common terms.
func TestKeywordOverlap(t *testing.T) {
	tests := []struct {
		name    string
		comment []string
		key     []string
		wantMin float64
	}{
		{"exact service match", []string{"stripe", "secret", "key"}, []string{"STRIPE", "SECRET", "KEY"}, 0.5},
		{"service expansion", []string{"openai", "key"}, []string{"OPENAI", "API", "KEY"}, 0.5},
		{"no overlap", []string{"random", "words"}, []string{"STRIPE", "KEY"}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := keywordOverlap(tt.comment, tt.key)
			if score < tt.wantMin {
				t.Errorf("score = %f, want >= %f", score, tt.wantMin)
			}
		})
	}
}
