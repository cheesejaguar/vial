package parser

import (
	"strings"
)

// Interpolate resolves variable references in a value string.
// Supports: ${VAR}, $VAR, ${VAR:-default}, ${VAR-default}
// The lookup function returns (value, found).
func Interpolate(value string, lookup func(key string) (string, bool)) string {
	var b strings.Builder
	b.Grow(len(value))

	i := 0
	for i < len(value) {
		if value[i] != '$' {
			b.WriteByte(value[i])
			i++
			continue
		}

		// We have a $
		if i+1 >= len(value) {
			b.WriteByte('$')
			i++
			continue
		}

		if value[i+1] == '{' {
			// ${VAR} or ${VAR:-default} or ${VAR-default}
			end := strings.Index(value[i:], "}")
			if end < 0 {
				// Unterminated — write literally
				b.WriteByte('$')
				i++
				continue
			}

			inner := value[i+2 : i+end]
			resolved := resolveRef(inner, lookup)
			b.WriteString(resolved)
			i += end + 1
		} else if isVarStart(value[i+1]) {
			// $VAR (simple form)
			j := i + 1
			for j < len(value) && isVarChar(value[j]) {
				j++
			}
			varName := value[i+1 : j]
			if val, ok := lookup(varName); ok {
				b.WriteString(val)
			}
			i = j
		} else {
			b.WriteByte('$')
			i++
		}
	}

	return b.String()
}

// resolveRef resolves an inner reference like "VAR", "VAR:-default", "VAR-default".
func resolveRef(inner string, lookup func(string) (string, bool)) string {
	// Check for :- (use default if unset or empty)
	if idx := strings.Index(inner, ":-"); idx >= 0 {
		varName := inner[:idx]
		defaultVal := inner[idx+2:]
		if val, ok := lookup(varName); ok && val != "" {
			return val
		}
		return defaultVal
	}

	// Check for - (use default if unset only)
	if idx := strings.Index(inner, "-"); idx >= 0 {
		varName := inner[:idx]
		defaultVal := inner[idx+1:]
		if val, ok := lookup(varName); ok {
			return val
		}
		return defaultVal
	}

	// Plain variable reference
	if val, ok := lookup(inner); ok {
		return val
	}
	return ""
}

func isVarStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func isVarChar(c byte) bool {
	return isVarStart(c) || (c >= '0' && c <= '9')
}
