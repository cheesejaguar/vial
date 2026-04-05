package matcher

import (
	"github.com/cheesejaguar/vial/internal/alias"
)

// AliasMatcher implements Tier 3: alias-based and auto-variant matching.
//
// This tier handles the common real-world situation where a project uses a
// shortened or alternative name for a secret that the vault stores under a
// different (often more explicit) key. Resolution happens in three passes,
// in descending confidence order:
//
//  1. Alias store lookup (confidence 0.90): the user has explicitly told Vial
//     that key A maps to vault key B via `vial label` or a pattern rule. This
//     is still capped below 1.0 because an alias may be stale after a vault
//     rename.
//
//  2. Forward variant detection (confidence 0.80): alias.DetectVariants expands
//     common _KEY / _API_KEY / _SECRET_KEY suffix substitutions. For example,
//     OPENAI_KEY → OPENAI_API_KEY.
//
//  3. Reverse variant detection (confidence 0.80): each vault key's variants are
//     checked against the requested key. This handles the mirror case where the
//     vault has the short form and the template uses the long form.
type AliasMatcher struct {
	Store *alias.Store // nil-safe: variant detection still runs without a store
}

// Tier returns 3, indicating this runs after normalization but before comment analysis.
func (m *AliasMatcher) Tier() int { return 3 }

// Name returns the short identifier for this tier.
func (m *AliasMatcher) Name() string { return "alias" }

// Match resolves requestedKey against vaultKeys using alias lookup and variant
// detection. It returns as soon as the first match is found, starting with the
// most authoritative source (alias store) and falling back to auto-detection.
func (m *AliasMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	// Build a set for O(1) vault membership checks used in passes 2 and 3.
	vaultKeySet := make(map[string]bool, len(vaultKeys))
	for _, vk := range vaultKeys {
		vaultKeySet[vk] = true
	}

	// Pass 1: explicit alias store (user-defined mappings + pattern rules).
	// The resolved canonical key may come from any of:
	//   - A user-defined alias set via `vial label`
	//   - A vault-loaded alias baked into the vault JSON
	//   - A regex PatternRule that matches the requested key
	if m.Store != nil {
		if canonical, ok := m.Store.Resolve(requestedKey); ok {
			if vaultKeySet[canonical] {
				return []MatchResult{{
					VaultKey:   canonical,
					Confidence: 0.90,
					Tier:       3,
					Reason:     "alias match",
				}}, nil
			}
		}
	}

	// Pass 2: forward — expand variants of the requested key and check vault.
	// DetectVariants generates plausible suffix transformations such as
	// FOO_KEY → FOO_API_KEY and FOO_KEY → FOO_SECRET_KEY.
	variants := alias.DetectVariants(requestedKey)
	for _, variant := range variants {
		if vaultKeySet[variant] {
			return []MatchResult{{
				VaultKey:   variant,
				Confidence: 0.80,
				Tier:       3,
				Reason:     "variant match (" + requestedKey + " → " + variant + ")",
			}}, nil
		}
	}

	// Pass 3: reverse — expand variants of each vault key and check if any
	// equals the requested key. This catches cases such as vault storing
	// OPENAI_KEY while the template requests OPENAI_API_KEY.
	for _, vk := range vaultKeys {
		variants := alias.DetectVariants(vk)
		for _, variant := range variants {
			if variant == requestedKey {
				return []MatchResult{{
					VaultKey:   vk,
					Confidence: 0.80,
					Tier:       3,
					Reason:     "reverse variant match (" + vk + " → " + requestedKey + ")",
				}}, nil
			}
		}
	}

	return nil, nil
}
