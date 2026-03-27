package parser

import (
	"bufio"
	"io"
	"strings"
)

// EnvEntry represents a parsed line from a .env file.
type EnvEntry struct {
	Key       string
	Value     string // parsed value (escapes resolved for double-quoted)
	RawValue  string // original value before interpolation
	Comment   string // inline or preceding comment text
	Line      int
	IsComment bool // full-line comment
	IsBlank   bool // blank line
	HasExport bool // had "export " prefix
	HasValue  bool // had = sign (distinguishes KEY= from KEY)
}

// Parse reads a .env or .env.example file and returns structured entries.
// Supports: unquoted, single-quoted (literal), double-quoted (with escapes),
// multi-line double-quoted, variable interpolation refs, export prefix, comments.
func Parse(r io.Reader) ([]EnvEntry, error) {
	var entries []EnvEntry
	scanner := bufio.NewScanner(r)
	lineNum := 0

	var multiLineEntry *EnvEntry
	var multiLineBuf strings.Builder

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// If we're in a multi-line double-quoted value, accumulate
		if multiLineEntry != nil {
			endIdx := strings.Index(line, "\"")
			if endIdx >= 0 {
				multiLineBuf.WriteString("\n")
				multiLineBuf.WriteString(line[:endIdx])
				multiLineEntry.Value = processDoubleQuotedEscapes(multiLineBuf.String())
				multiLineEntry.RawValue = multiLineBuf.String()

				// Check for inline comment after closing quote
				rest := strings.TrimSpace(line[endIdx+1:])
				if strings.HasPrefix(rest, "#") {
					multiLineEntry.Comment = strings.TrimSpace(rest[1:])
				}

				entries = append(entries, *multiLineEntry)
				multiLineEntry = nil
				multiLineBuf.Reset()
			} else {
				multiLineBuf.WriteString("\n")
				multiLineBuf.WriteString(line)
			}
			continue
		}

		// Blank line
		if strings.TrimSpace(line) == "" {
			entries = append(entries, EnvEntry{Line: lineNum, IsBlank: true})
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Full-line comment
		if strings.HasPrefix(trimmed, "#") {
			entries = append(entries, EnvEntry{
				Line:      lineNum,
				IsComment: true,
				Comment:   strings.TrimPrefix(trimmed, "#"),
			})
			continue
		}

		entry := EnvEntry{Line: lineNum}

		// Strip export prefix
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimPrefix(trimmed, "export ")
			entry.HasExport = true
		}

		// Split on first =
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			entry.Key = strings.TrimSpace(trimmed)
			entries = append(entries, entry)
			continue
		}

		entry.Key = strings.TrimSpace(trimmed[:eqIdx])
		entry.HasValue = true
		rawValue := trimmed[eqIdx+1:]

		// Parse the value
		val, comment, isMultiLine := parseValueFull(rawValue)
		if isMultiLine {
			multiLineEntry = &entry
			multiLineBuf.WriteString(val)
			continue
		}

		entry.Value = val
		entry.RawValue = val
		entry.Comment = comment
		entries = append(entries, entry)
	}

	// Handle unterminated multi-line (best effort)
	if multiLineEntry != nil {
		multiLineEntry.Value = processDoubleQuotedEscapes(multiLineBuf.String())
		multiLineEntry.RawValue = multiLineBuf.String()
		entries = append(entries, *multiLineEntry)
	}

	return entries, scanner.Err()
}

// parseValueFull extracts value, comment, and whether it's an unterminated multi-line.
func parseValueFull(raw string) (value, comment string, isMultiLine bool) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return "", "", false
	}

	// Double-quoted value with escape support
	if strings.HasPrefix(raw, "\"") {
		content := raw[1:]
		// Find the closing quote, respecting escaped quotes
		endIdx := findClosingQuote(content)
		if endIdx < 0 {
			// Unterminated — start of multi-line
			return content, "", true
		}
		innerRaw := content[:endIdx]
		value = processDoubleQuotedEscapes(innerRaw)

		rest := strings.TrimSpace(content[endIdx+1:])
		if strings.HasPrefix(rest, "#") {
			comment = strings.TrimSpace(rest[1:])
		}
		return value, comment, false
	}

	// Single-quoted value (literal, no escapes, no interpolation)
	if strings.HasPrefix(raw, "'") {
		end := strings.Index(raw[1:], "'")
		if end >= 0 {
			value = raw[1 : end+1]
			rest := strings.TrimSpace(raw[end+2:])
			if strings.HasPrefix(rest, "#") {
				comment = strings.TrimSpace(rest[1:])
			}
			return value, comment, false
		}
		// Unterminated single quote — treat as literal
		return raw[1:], "", false
	}

	// Unquoted value: take until " #" (inline comment marker)
	commentIdx := strings.Index(raw, " #")
	if commentIdx >= 0 {
		value = strings.TrimSpace(raw[:commentIdx])
		comment = strings.TrimSpace(raw[commentIdx+2:])
		return value, comment, false
	}

	return strings.TrimSpace(raw), "", false
}

// findClosingQuote finds the index of the closing " in a double-quoted string,
// respecting backslash escapes. Returns -1 if not found.
func findClosingQuote(s string) int {
	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return -1
}

// processDoubleQuotedEscapes handles escape sequences in double-quoted values.
func processDoubleQuotedEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case '$':
				b.WriteByte('$')
			default:
				// Unknown escape — keep as-is
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		b.WriteByte(s[i])
	}
	if escaped {
		b.WriteByte('\\') // trailing backslash
	}
	return b.String()
}

// KeysNeeded extracts just the variable names that need resolution from parsed entries.
func KeysNeeded(entries []EnvEntry) []string {
	var keys []string
	for _, e := range entries {
		if e.Key != "" && !e.IsComment && !e.IsBlank {
			keys = append(keys, e.Key)
		}
	}
	return keys
}
