package parser

import (
	"strings"
)

// Interpolate performs shell-style variable expansion on value, replacing
// references to environment variables using the provided lookup function.
//
// Supported reference forms:
//
//	${VAR}          — replaced with the value of VAR; empty string if unset
//	${VAR:-default} — replaced with VAR's value if set and non-empty, else default
//	${VAR-default}  — replaced with VAR's value if set (even if empty), else default
//	$VAR            — simple form; the variable name ends at the first non-identifier char
//
// The lookup function receives the variable name and returns (value, found).
// If found is false the reference expands to an empty string (unless a default
// is specified via :- or -). An unterminated "${" sequence (no closing "}")
// is emitted literally rather than silently dropped.
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

		// Current character is '$'. Peek at the next character to determine
		// which reference form we are dealing with.
		if i+1 >= len(value) {
			// '$' at the very end of the string — emit literally.
			b.WriteByte('$')
			i++
			continue
		}

		if value[i+1] == '{' {
			// Braced form: ${VAR}, ${VAR:-default}, or ${VAR-default}.
			end := strings.Index(value[i:], "}")
			if end < 0 {
				// No closing brace — emit the '$' literally and continue so
				// the rest of the string is still processed character by character.
				b.WriteByte('$')
				i++
				continue
			}

			// inner is the content between the braces, e.g. "VAR:-default".
			inner := value[i+2 : i+end]
			resolved := resolveRef(inner, lookup)
			b.WriteString(resolved)
			i += end + 1
		} else if isVarStart(value[i+1]) {
			// Simple $VAR form: consume identifier characters after the '$'.
			j := i + 1
			for j < len(value) && isVarChar(value[j]) {
				j++
			}
			varName := value[i+1 : j]
			if val, ok := lookup(varName); ok {
				b.WriteString(val)
			}
			// If the variable is not found, expand to empty string (omit it).
			i = j
		} else {
			// '$' followed by a non-identifier, non-brace character (e.g. "$ ")
			// — emit the '$' literally.
			b.WriteByte('$')
			i++
		}
	}

	return b.String()
}

// resolveRef resolves the content inside a "${...}" reference, which may be
// a plain variable name or one of the default-value forms:
//
//   - "VAR:-default": return VAR's value if it is set and non-empty; else default.
//     This is the most common default form and mirrors POSIX shell "${VAR:-word}".
//   - "VAR-default": return VAR's value if the variable is set at all (even to
//     the empty string); else default. Mirrors POSIX "${VAR-word}".
//   - "VAR": plain lookup; empty string if not found.
//
// The ":-" form is checked first because it contains "-" as a substring; a
// simple strings.Index for "-" would match the wrong position in "VAR:-def".
func resolveRef(inner string, lookup func(string) (string, bool)) string {
	// Check for :- (use default when unset OR empty).
	if idx := strings.Index(inner, ":-"); idx >= 0 {
		varName := inner[:idx]
		defaultVal := inner[idx+2:]
		if val, ok := lookup(varName); ok && val != "" {
			return val
		}
		return defaultVal
	}

	// Check for - (use default only when unset, not when empty).
	if idx := strings.Index(inner, "-"); idx >= 0 {
		varName := inner[:idx]
		defaultVal := inner[idx+1:]
		if val, ok := lookup(varName); ok {
			return val
		}
		return defaultVal
	}

	// Plain variable reference — empty string if not found.
	if val, ok := lookup(inner); ok {
		return val
	}
	return ""
}

// isVarStart reports whether c is a valid first character for an environment
// variable name: an ASCII letter or underscore. Digits are not valid first
// characters, consistent with POSIX variable naming rules.
func isVarStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

// isVarChar reports whether c is a valid continuation character for an
// environment variable name: an ASCII letter, digit, or underscore.
func isVarChar(c byte) bool {
	return isVarStart(c) || (c >= '0' && c <= '9')
}
