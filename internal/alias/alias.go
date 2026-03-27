package alias

import (
	"strings"
	"sync"
)

// Store manages the mapping of alias names to canonical vault key names.
type Store struct {
	mu       sync.RWMutex
	aliases  map[string]string // alias → canonical key
	patterns []PatternRule     // regex-based rules
}

// NewStore creates a new alias store.
func NewStore() *Store {
	return &Store{
		aliases: make(map[string]string),
	}
}

// Set maps an alias name to a canonical vault key.
func (s *Store) Set(alias, canonical string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliases[alias] = canonical
}

// Get returns the canonical key for an alias, if it exists.
func (s *Store) Get(alias string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	canonical, ok := s.aliases[alias]
	return canonical, ok
}

// Remove deletes an alias mapping.
func (s *Store) Remove(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.aliases, alias)
}

// List returns all alias→canonical mappings.
func (s *Store) List() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.aliases))
	for k, v := range s.aliases {
		result[k] = v
	}
	return result
}

// AliasesFor returns all aliases that map to the given canonical key.
func (s *Store) AliasesFor(canonical string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []string
	for alias, key := range s.aliases {
		if key == canonical {
			result = append(result, alias)
		}
	}
	return result
}

// Resolve looks up a key through aliases and patterns.
// Returns (canonical_key, found).
func (s *Store) Resolve(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Direct alias lookup
	if canonical, ok := s.aliases[key]; ok {
		return canonical, true
	}

	// Pattern-based matching
	for _, p := range s.patterns {
		if p.Matches(key) {
			return p.MapsTo, true
		}
	}

	return "", false
}

// AddPattern adds a regex-based alias rule.
func (s *Store) AddPattern(rule PatternRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns = append(s.patterns, rule)
}

// Patterns returns all pattern rules.
func (s *Store) Patterns() []PatternRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PatternRule, len(s.patterns))
	copy(result, s.patterns)
	return result
}

// LoadFromVault populates the store from vault secret entries' alias lists.
func (s *Store) LoadFromVault(keyAliases map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for canonical, aliases := range keyAliases {
		for _, alias := range aliases {
			s.aliases[alias] = canonical
		}
	}
}

// Normalize converts a key name to a normalized form for comparison.
// Strips common framework prefixes, lowercases, normalizes separators.
func Normalize(key string) string {
	k := strings.ToUpper(key)

	// Strip common framework prefixes
	for _, prefix := range frameworkPrefixes {
		if strings.HasPrefix(k, prefix) {
			k = strings.TrimPrefix(k, prefix)
			break
		}
	}

	return k
}

var frameworkPrefixes = []string{
	"NEXT_PUBLIC_",
	"VITE_",
	"REACT_APP_",
	"NUXT_PUBLIC_",
	"GATSBY_",
	"VUE_APP_",
	"EXPO_PUBLIC_",
}
