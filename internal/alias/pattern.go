package alias

import (
	"fmt"
	"regexp"
)

// PatternRule maps a regex pattern to a canonical vault key.
type PatternRule struct {
	Pattern string         // regex pattern string
	MapsTo  string         // canonical vault key name
	re      *regexp.Regexp // compiled regex
}

// NewPatternRule creates and compiles a pattern rule.
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

// Matches returns true if the key matches this pattern.
func (p *PatternRule) Matches(key string) bool {
	if p.re == nil {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return false
		}
		p.re = re
	}
	return p.re.MatchString(key)
}
