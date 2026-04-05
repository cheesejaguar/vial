// Package alias implements the user-defined alias store and framework-prefix
// normalization used by the matcher's tier-3 (alias/variant) resolution layer.
//
// The store holds two kinds of mappings:
//
//   - Exact aliases: a named string maps to a canonical vault key.
//     These are set explicitly by the user (e.g. "OAI_KEY" → "OPENAI_API_KEY").
//
//   - Pattern rules: a compiled regex maps to a canonical vault key.
//     These let a single rule cover many key name variations.
//
// Resolve checks exact aliases first, then patterns in insertion order.
// All methods are safe for concurrent use.
package alias

import (
	"strings"
	"sync"
)

// Store manages the mapping of alias names to canonical vault key names.
// The zero value is not usable; call NewStore.
type Store struct {
	mu       sync.RWMutex
	aliases  map[string]string // alias → canonical key
	patterns []PatternRule     // regex-based rules, checked when no exact alias matches
}

// NewStore creates a new, empty alias store.
func NewStore() *Store {
	return &Store{
		aliases: make(map[string]string),
	}
}

// Set maps an alias name to a canonical vault key.
// Overwrites any previous mapping for the same alias name.
func (s *Store) Set(alias, canonical string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliases[alias] = canonical
}

// Get returns the canonical key for an exact alias lookup.
// The second return value is false when the alias is not present.
func (s *Store) Get(alias string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	canonical, ok := s.aliases[alias]
	return canonical, ok
}

// Remove deletes an alias mapping. It is a no-op if the alias does not exist.
func (s *Store) Remove(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.aliases, alias)
}

// List returns a snapshot of all alias → canonical mappings.
// The returned map is a copy; mutations do not affect the store.
func (s *Store) List() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.aliases))
	for k, v := range s.aliases {
		result[k] = v
	}
	return result
}

// AliasesFor returns all alias names that map to the given canonical key.
// The order of results is not guaranteed.
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

// Resolve looks up a key through aliases and pattern rules, in that order.
// It returns the canonical vault key name and true when found, or ("", false)
// when no alias or pattern matches.
func (s *Store) Resolve(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Direct alias lookup — checked first because it is O(1) and deterministic.
	if canonical, ok := s.aliases[key]; ok {
		return canonical, true
	}

	// Pattern-based matching — evaluated in insertion order so more specific
	// patterns added first take precedence.
	for _, p := range s.patterns {
		if p.Matches(key) {
			return p.MapsTo, true
		}
	}

	return "", false
}

// AddPattern appends a compiled regex rule to the pattern list.
// Patterns are tested in insertion order during Resolve.
func (s *Store) AddPattern(rule PatternRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns = append(s.patterns, rule)
}

// Patterns returns a snapshot copy of all registered pattern rules.
func (s *Store) Patterns() []PatternRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PatternRule, len(s.patterns))
	copy(result, s.patterns)
	return result
}

// LoadFromVault populates the store in bulk from a vault's alias metadata.
// The argument maps each canonical key name to the list of aliases stored
// alongside it in the vault JSON. Called at vault-unlock time so that all
// user-persisted aliases are immediately available for matching.
func (s *Store) LoadFromVault(keyAliases map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for canonical, aliases := range keyAliases {
		for _, alias := range aliases {
			s.aliases[alias] = canonical
		}
	}
}

// Normalize converts a key name to a canonical comparison form.
// It uppercases the key and strips any recognized framework prefix
// (NEXT_PUBLIC_, VITE_, REACT_APP_, etc.) so that framework-namespaced
// keys can be matched against their un-prefixed vault counterparts.
// Only the first matching prefix is stripped.
func Normalize(key string) string {
	k := strings.ToUpper(key)

	// Strip common framework prefixes that wrap an otherwise identical key.
	// For example, NEXT_PUBLIC_STRIPE_KEY → STRIPE_KEY.
	for _, prefix := range frameworkPrefixes {
		if strings.HasPrefix(k, prefix) {
			k = strings.TrimPrefix(k, prefix)
			break
		}
	}

	return k
}

// frameworkPrefixes lists the environment-variable prefixes injected by
// popular JavaScript build tools. The matcher strips these before comparing
// a template key to vault keys, since both refer to the same underlying
// secret regardless of which framework prefix wraps it.
var frameworkPrefixes = []string{
	"NEXT_PUBLIC_",
	"VITE_",
	"REACT_APP_",
	"NUXT_PUBLIC_",
	"GATSBY_",
	"VUE_APP_",
	"EXPO_PUBLIC_",
}
