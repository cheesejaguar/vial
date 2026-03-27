package matcher

import (
	"github.com/cheesejaguar/vial/internal/alias"
)

// AliasMatcher implements Tier 3: alias-based and variant-based matching.
// It uses both the alias store (user-defined + vault aliases + pattern rules)
// and auto-detected variants.
type AliasMatcher struct {
	Store *alias.Store
}

func (m *AliasMatcher) Tier() int    { return 3 }
func (m *AliasMatcher) Name() string { return "alias" }

func (m *AliasMatcher) Match(requestedKey string, vaultKeys []string) ([]MatchResult, error) {
	vaultKeySet := make(map[string]bool, len(vaultKeys))
	for _, vk := range vaultKeys {
		vaultKeySet[vk] = true
	}

	// 1. Check alias store (user-defined aliases + vault-loaded aliases + pattern rules)
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

	// 2. Auto-detect variants of the requested key and check if any exist in vault
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

	// 3. Reverse: check if any vault key's variants match the requested key
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
