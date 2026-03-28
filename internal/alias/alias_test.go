package alias

import (
	"testing"
)

func TestStoreSetGetResolve(t *testing.T) {
	s := NewStore()

	s.Set("OPENAI_KEY", "OPENAI_API_KEY")
	s.Set("OAI_KEY", "OPENAI_API_KEY")

	// Direct lookup
	if v, ok := s.Get("OPENAI_KEY"); !ok || v != "OPENAI_API_KEY" {
		t.Errorf("Get(OPENAI_KEY) = %q, %v", v, ok)
	}

	// Resolve
	if v, ok := s.Resolve("OAI_KEY"); !ok || v != "OPENAI_API_KEY" {
		t.Errorf("Resolve(OAI_KEY) = %q, %v", v, ok)
	}

	// Not found
	if _, ok := s.Resolve("MISSING"); ok {
		t.Error("expected Resolve(MISSING) to return false")
	}
}

func TestStoreAliasesFor(t *testing.T) {
	s := NewStore()
	s.Set("OPENAI_KEY", "OPENAI_API_KEY")
	s.Set("OAI_KEY", "OPENAI_API_KEY")
	s.Set("STRIPE_KEY", "STRIPE_SECRET_KEY")

	aliases := s.AliasesFor("OPENAI_API_KEY")
	if len(aliases) != 2 {
		t.Fatalf("got %d aliases, want 2", len(aliases))
	}
}

func TestStoreRemove(t *testing.T) {
	s := NewStore()
	s.Set("ALIAS", "CANONICAL")
	s.Remove("ALIAS")

	if _, ok := s.Get("ALIAS"); ok {
		t.Error("alias should have been removed")
	}
}

func TestStorePatternRule(t *testing.T) {
	s := NewStore()

	rule, err := NewPatternRule(`.*OPENAI.*KEY.*`, "OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("NewPatternRule: %v", err)
	}
	s.AddPattern(rule)

	if v, ok := s.Resolve("MY_OPENAI_SECRET_KEY"); !ok || v != "OPENAI_API_KEY" {
		t.Errorf("Resolve(MY_OPENAI_SECRET_KEY) = %q, %v", v, ok)
	}

	// Non-matching
	if _, ok := s.Resolve("STRIPE_KEY"); ok {
		t.Error("STRIPE_KEY should not match OPENAI pattern")
	}
}

func TestStoreLoadFromVault(t *testing.T) {
	s := NewStore()
	s.LoadFromVault(map[string][]string{
		"OPENAI_API_KEY":    {"OPENAI_KEY", "OAI_KEY"},
		"STRIPE_SECRET_KEY": {"STRIPE_KEY"},
	})

	if v, ok := s.Resolve("OPENAI_KEY"); !ok || v != "OPENAI_API_KEY" {
		t.Errorf("Resolve(OPENAI_KEY) = %q, %v", v, ok)
	}
	if v, ok := s.Resolve("STRIPE_KEY"); !ok || v != "STRIPE_SECRET_KEY" {
		t.Errorf("Resolve(STRIPE_KEY) = %q, %v", v, ok)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"NEXT_PUBLIC_SUPABASE_URL", "SUPABASE_URL"},
		{"VITE_API_KEY", "API_KEY"},
		{"REACT_APP_STRIPE_KEY", "STRIPE_KEY"},
		{"OPENAI_API_KEY", "OPENAI_API_KEY"},
		{"lowercase_key", "LOWERCASE_KEY"},
	}

	for _, tt := range tests {
		got := Normalize(tt.input)
		if got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDetectVariants(t *testing.T) {
	tests := []struct {
		key     string
		wantAny []string // at least these should appear
	}{
		{"OPENAI_KEY", []string{"OPENAI_API_KEY", "OPENAI_SECRET_KEY"}},
		{"OPENAI_API_KEY", []string{"OPENAI_KEY", "OPENAI_SECRET_KEY"}},
		{"DB_PASSWORD", []string{"DB_PASS", "DB_PW"}},
		{"MAPBOX_TOKEN", []string{"MAPBOX_ACCESS_TOKEN", "MAPBOX_AUTH_TOKEN", "MAPBOX_BEARER_TOKEN"}},
		{"API_URL", []string{"API_URI", "API_ENDPOINT"}},
		{"NEXT_PUBLIC_SUPABASE_KEY", []string{"SUPABASE_KEY", "SUPABASE_API_KEY", "SUPABASE_SECRET_KEY"}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			variants := DetectVariants(tt.key)
			varSet := make(map[string]bool)
			for _, v := range variants {
				varSet[v] = true
			}
			for _, want := range tt.wantAny {
				if !varSet[want] {
					t.Errorf("DetectVariants(%q) missing %q (got %v)", tt.key, want, variants)
				}
			}
			// Should not contain the original key
			if varSet[tt.key] {
				t.Errorf("DetectVariants(%q) should not contain itself", tt.key)
			}
		})
	}
}

func TestPatternRuleInvalid(t *testing.T) {
	_, err := NewPatternRule("[invalid", "TARGET")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
