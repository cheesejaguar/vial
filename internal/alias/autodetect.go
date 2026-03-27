package alias

import "strings"

// VariantRule describes common naming variations for API keys.
type VariantRule struct {
	Suffixes []string // e.g., ["_KEY", "_API_KEY", "_SECRET_KEY"]
}

// builtinVariantRules maps service-agnostic suffix groups.
// If a vault has "FOO_API_KEY" and a template asks for "FOO_KEY", these rules
// let us recognize them as the same thing.
var builtinVariantRules = []VariantRule{
	{Suffixes: []string{"_KEY", "_API_KEY", "_SECRET_KEY"}},
	{Suffixes: []string{"_TOKEN", "_ACCESS_TOKEN", "_AUTH_TOKEN", "_BEARER_TOKEN"}},
	{Suffixes: []string{"_SECRET", "_SECRET_KEY"}},
	{Suffixes: []string{"_PASSWORD", "_PASS", "_PW"}},
	{Suffixes: []string{"_URL", "_URI", "_ENDPOINT"}},
	{Suffixes: []string{"_HOST", "_HOSTNAME", "_SERVER"}},
}

// DetectVariants finds potential variant names for a given key based on built-in rules.
// For example, "OPENAI_KEY" would generate ["OPENAI_API_KEY", "OPENAI_SECRET_KEY"].
func DetectVariants(key string) []string {
	key = strings.ToUpper(key)
	var variants []string

	for _, rule := range builtinVariantRules {
		// Try longest suffixes first so "_API_KEY" matches before "_KEY"
		bestSuffix := ""
		for _, suffix := range rule.Suffixes {
			if strings.HasSuffix(key, suffix) && len(suffix) > len(bestSuffix) {
				bestSuffix = suffix
			}
		}
		if bestSuffix != "" {
			base := strings.TrimSuffix(key, bestSuffix)
			for _, otherSuffix := range rule.Suffixes {
				variant := base + otherSuffix
				if variant != key {
					variants = append(variants, variant)
				}
			}
			break // only use the first matching rule
		}
	}

	// Also try stripping framework prefixes
	for _, prefix := range frameworkPrefixes {
		if strings.HasPrefix(key, prefix) {
			stripped := strings.TrimPrefix(key, prefix)
			variants = append(variants, stripped)
			// Also generate suffix variants of the stripped version
			for _, v := range DetectVariants(stripped) {
				variants = append(variants, v)
			}
			break
		}
	}

	return unique(variants)
}

func unique(s []string) []string {
	seen := make(map[string]bool, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
