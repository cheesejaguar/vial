package alias

import "strings"

// VariantRule describes a group of suffixes that are semantically equivalent
// for a given class of credential (API key, token, password, etc.).
// When a vault key matches any suffix in the group, DetectVariants generates
// alternative names using every other suffix in the same group.
type VariantRule struct {
	Suffixes []string // e.g., ["_KEY", "_API_KEY", "_SECRET_KEY"]
}

// builtinVariantRules lists the service-agnostic suffix equivalence groups.
// If a vault has "FOO_API_KEY" and a template asks for "FOO_KEY", these rules
// let the matcher recognize them as referring to the same credential without
// any explicit user configuration.
var builtinVariantRules = []VariantRule{
	{Suffixes: []string{"_KEY", "_API_KEY", "_SECRET_KEY"}},
	{Suffixes: []string{"_TOKEN", "_ACCESS_TOKEN", "_AUTH_TOKEN", "_BEARER_TOKEN"}},
	{Suffixes: []string{"_SECRET", "_SECRET_KEY"}},
	{Suffixes: []string{"_PASSWORD", "_PASS", "_PW"}},
	{Suffixes: []string{"_URL", "_URI", "_ENDPOINT"}},
	{Suffixes: []string{"_HOST", "_HOSTNAME", "_SERVER"}},
}

// DetectVariants returns a deduplicated list of alternative key names for the
// given key, derived from the built-in suffix equivalence rules and framework
// prefix stripping.
//
// Examples:
//
//	DetectVariants("OPENAI_KEY")          → ["OPENAI_API_KEY", "OPENAI_SECRET_KEY"]
//	DetectVariants("NEXT_PUBLIC_DB_URL")  → ["DB_URL", "DB_URI", "DB_ENDPOINT", ...]
//
// The original key is never included in the result.
func DetectVariants(key string) []string {
	key = strings.ToUpper(key)
	var variants []string

	for _, rule := range builtinVariantRules {
		// Find the longest matching suffix so "_API_KEY" takes precedence over
		// "_KEY" when both are present in the same rule group.
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
			// Only the first matching rule group applies; a key cannot belong
			// to two different credential classes simultaneously.
			break
		}
	}

	// Strip framework prefixes and recursively generate suffix variants of the
	// unprefixed form so callers get the full expansion in one call.
	for _, prefix := range frameworkPrefixes {
		if strings.HasPrefix(key, prefix) {
			stripped := strings.TrimPrefix(key, prefix)
			variants = append(variants, stripped)
			// Also generate suffix variants of the stripped version so that
			// e.g. NEXT_PUBLIC_SUPABASE_KEY → SUPABASE_API_KEY is surfaced.
			for _, v := range DetectVariants(stripped) {
				variants = append(variants, v)
			}
			break
		}
	}

	return unique(variants)
}

// unique filters a string slice to remove duplicates while preserving order.
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
