package alias

import (
	"fmt"
	"regexp"
)

// PatternRule maps a regular-expression pattern to a canonical vault key name.
// It is used when a user wants a flexible, wildcard-style alias rather than
// an exact string mapping — for example, routing any key that matches
// `.*OPENAI.*KEY.*` to "OPENAI_API_KEY".
type PatternRule struct {
	Pattern string         // original regex string, stored for serialization
	MapsTo  string         // canonical vault key name this pattern resolves to
	re      *regexp.Regexp // compiled form of Pattern; lazily initialized if nil
}

// NewPatternRule compiles pattern into a PatternRule that maps to mapsTo.
// Returns an error if pattern is not a valid regular expression.
func NewPatternRule(pattern, mapsTo string) (PatternRule, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return PatternRule{}, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	return PatternRule{
		Pattern: pattern,
		MapsTo:  mapsTo,
		re:      re,
	}, nil
}

// Matches reports whether key is matched by this pattern rule.
// If the compiled regex is missing (e.g. after JSON round-tripping), it is
// recompiled on the fly; a compilation failure is treated as no-match rather
// than a hard error so that a single bad persisted rule cannot break resolution
// for all other keys.
func (p *PatternRule) Matches(key string) bool {
	if p.re == nil {
		// Lazily compile in case the rule was deserialized without the re field.
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return false
		}
		p.re = re
	}
	return p.re.MatchString(key)
}
