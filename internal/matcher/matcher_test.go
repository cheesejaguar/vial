package matcher

import (
	"testing"

	"github.com/cheesejaguar/vial/internal/alias"
)

// --- Tier 1: Exact ---

func TestExactMatcherFound(t *testing.T) {
	m := &ExactMatcher{}
	results, err := m.Match("OPENAI_API_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].VaultKey != "OPENAI_API_KEY" {
		t.Errorf("VaultKey = %q, want OPENAI_API_KEY", results[0].VaultKey)
	}
	if results[0].Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", results[0].Confidence)
	}
}

func TestExactMatcherNotFound(t *testing.T) {
	m := &ExactMatcher{}
	results, err := m.Match("MISSING_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if results != nil {
		t.Errorf("got results %v, want nil", results)
	}
}

func TestExactMatcherCaseSensitive(t *testing.T) {
	m := &ExactMatcher{}
	results, _ := m.Match("openai_api_key", []string{"OPENAI_API_KEY"})
	if results != nil {
		t.Error("case-different key should not match in tier 1")
	}
}

// --- Tier 2: Normalize ---

func TestNormalizeMatcherPrefixStrip(t *testing.T) {
	m := &NormalizeMatcher{}
	tests := []struct {
		name     string
		reqKey   string
		vault    []string
		wantKey  string
		wantTier int
	}{
		{"NEXT_PUBLIC prefix", "NEXT_PUBLIC_SUPABASE_URL", []string{"SUPABASE_URL"}, "SUPABASE_URL", 2},
		{"VITE prefix", "VITE_API_KEY", []string{"API_KEY"}, "API_KEY", 2},
		{"REACT_APP prefix", "REACT_APP_STRIPE_KEY", []string{"STRIPE_KEY"}, "STRIPE_KEY", 2},
		{"reverse: vault has prefix", "SUPABASE_URL", []string{"NEXT_PUBLIC_SUPABASE_URL"}, "NEXT_PUBLIC_SUPABASE_URL", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := m.Match(tt.reqKey, tt.vault)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			if len(results) == 0 {
				t.Fatal("expected a match")
			}
			if results[0].VaultKey != tt.wantKey {
				t.Errorf("VaultKey = %q, want %q", results[0].VaultKey, tt.wantKey)
			}
			if results[0].Tier != tt.wantTier {
				t.Errorf("Tier = %d, want %d", results[0].Tier, tt.wantTier)
			}
		})
	}
}

func TestNormalizeMatcherNoMatch(t *testing.T) {
	m := &NormalizeMatcher{}
	results, _ := m.Match("COMPLETELY_DIFFERENT", []string{"OPENAI_API_KEY"})
	if len(results) != 0 {
		t.Errorf("expected no matches, got %v", results)
	}
}

// --- Tier 3: Alias ---

func TestAliasMatcherUserDefined(t *testing.T) {
	store := alias.NewStore()
	store.Set("OPENAI_KEY", "OPENAI_API_KEY")

	m := &AliasMatcher{Store: store}
	results, err := m.Match("OPENAI_KEY", []string{"OPENAI_API_KEY", "STRIPE_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a match")
	}
	if results[0].VaultKey != "OPENAI_API_KEY" {
		t.Errorf("VaultKey = %q, want OPENAI_API_KEY", results[0].VaultKey)
	}
	if results[0].Tier != 3 {
		t.Errorf("Tier = %d, want 3", results[0].Tier)
	}
}

func TestAliasMatcherPatternRule(t *testing.T) {
	store := alias.NewStore()
	rule, _ := alias.NewPatternRule(`.*STRIPE.*SECRET.*`, "STRIPE_SECRET_KEY")
	store.AddPattern(rule)

	m := &AliasMatcher{Store: store}
	results, err := m.Match("MY_STRIPE_SECRET_VALUE", []string{"STRIPE_SECRET_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a match via pattern")
	}
	if results[0].VaultKey != "STRIPE_SECRET_KEY" {
		t.Errorf("VaultKey = %q, want STRIPE_SECRET_KEY", results[0].VaultKey)
	}
}

func TestAliasMatcherAutoVariant(t *testing.T) {
	m := &AliasMatcher{Store: alias.NewStore()}

	// OPENAI_KEY should auto-detect OPENAI_API_KEY as a variant
	results, err := m.Match("OPENAI_KEY", []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a variant match")
	}
	if results[0].VaultKey != "OPENAI_API_KEY" {
		t.Errorf("VaultKey = %q, want OPENAI_API_KEY", results[0].VaultKey)
	}
	if results[0].Confidence != 0.80 {
		t.Errorf("Confidence = %f, want 0.80", results[0].Confidence)
	}
}

func TestAliasMatcherReverseVariant(t *testing.T) {
	m := &AliasMatcher{Store: alias.NewStore()}

	// Template asks for OPENAI_API_KEY, vault has OPENAI_KEY
	results, err := m.Match("OPENAI_API_KEY", []string{"OPENAI_KEY"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a reverse variant match")
	}
	if results[0].VaultKey != "OPENAI_KEY" {
		t.Errorf("VaultKey = %q, want OPENAI_KEY", results[0].VaultKey)
	}
}

// --- Chain ---

func TestChainTierPriority(t *testing.T) {
	store := alias.NewStore()
	store.Set("ALIAS_KEY", "VAULT_KEY")

	chain := NewChain(
		&ExactMatcher{},
		&NormalizeMatcher{},
		&AliasMatcher{Store: store},
	)

	// Exact match should win
	result, err := chain.Resolve("VAULT_KEY", []string{"VAULT_KEY"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Tier != 1 {
		t.Errorf("expected tier 1, got %d", result.Tier)
	}

	// Alias match when no exact match
	result, err = chain.Resolve("ALIAS_KEY", []string{"VAULT_KEY"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Tier != 3 {
		t.Errorf("expected tier 3, got %d", result.Tier)
	}
}

func TestChainReturnsNilWhenNoMatch(t *testing.T) {
	chain := NewChain(&ExactMatcher{}, &NormalizeMatcher{})
	result, _ := chain.Resolve("TOTALLY_DIFFERENT", []string{"OPENAI_API_KEY"})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestChainResolveAll(t *testing.T) {
	store := alias.NewStore()
	chain := NewChain(
		&ExactMatcher{},
		&NormalizeMatcher{},
		&AliasMatcher{Store: store},
	)

	results, err := chain.ResolveAll("OPENAI_KEY", []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	// Should get results from tier 3 (variant match)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
}
