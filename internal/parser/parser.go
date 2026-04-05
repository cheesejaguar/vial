// Package parser reads and writes .env files while preserving their structure.
//
// The parser handles the full surface area of the de-facto .env format:
//   - Unquoted values (terminated by optional " #" inline comment)
//   - Single-quoted values (literal — no escape sequences, no interpolation)
//   - Double-quoted values (backslash escapes: \n \t \r \\ \" \$)
//   - Multi-line double-quoted values (opening quote on one line, closing on a later line)
//   - The "export " key prefix emitted by some shell-centric generators
//   - Inline comments on key=value lines and standalone full-line comments
//   - Blank lines, so that [WriteEnvFile] can round-trip layout faithfully
//
// Parsed entries are represented as [EnvEntry] values, which carry enough
// metadata to reconstruct the original file with substituted secret values.
package parser

import (
	"bufio"
	"io"
	"strings"
)

// EnvEntry represents a single parsed line from a .env file.
//
// Blank lines and comment lines are represented with IsBlank/IsComment set
// rather than populating Key and Value. This allows [WriteEnvFile] to
// reconstruct the file layout exactly, including spacing and commentary.
type EnvEntry struct {
	Key      string // environment variable name (empty for comment/blank lines)
	Value    string // parsed value with escape sequences resolved (double-quoted only)
	RawValue string // original value text before escape processing, used for round-trip fidelity

	// Comment holds the comment text without the leading "#".
	// For full-line comments this is the entire comment body; for inline
	// comments it is the text after " # " on a key=value line.
	Comment string

	Line      int  // 1-based line number in the source file
	IsComment bool // true when the entire line is a comment (starts with #)
	IsBlank   bool // true when the line contains only whitespace

	// HasExport is true when the original line used "export KEY=value" syntax.
	// Some tools (e.g. shell scripts) generate this form; we record it so
	// callers can be aware, though WriteEnvFile normalises output without it.
	HasExport bool

	// HasValue distinguishes "KEY=" (empty value, HasValue=true) from "KEY"
	// (no equals sign, HasValue=false). Both are valid in .env files, but
	// they convey different intent — the latter is a bare declaration.
	HasValue bool
}

// Parse reads a .env or .env.example file from r and returns one [EnvEntry]
// per logical line. The parser is permissive by design: it never returns an
// error for malformed syntax and instead makes a best-effort interpretation
// so that real-world files with inconsistencies are still usable.
//
// Supported syntax:
//   - Unquoted: KEY=value            (comment after " #")
//   - Single-quoted: KEY='value'     (no escapes; # inside is literal)
//   - Double-quoted: KEY="value"     (\n \t \r \\ \" \$ escapes)
//   - Multi-line double-quoted spanning several physical lines
//   - export KEY=value
//   - # full-line comment
//   - KEY (no equals sign — bare declaration)
func Parse(r io.Reader) ([]EnvEntry, error) {
	var entries []EnvEntry
	scanner := bufio.NewScanner(r)
	lineNum := 0

	// multiLineEntry holds a partially-built entry whose double-quoted value
	// has not yet been closed. Lines are accumulated in multiLineBuf until
	// we encounter the matching closing double-quote.
	var multiLineEntry *EnvEntry
	var multiLineBuf strings.Builder

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Continuation mode: we are inside an unterminated double-quoted value.
		// Scan each line until we find the closing quote character.
		if multiLineEntry != nil {
			endIdx := strings.Index(line, "\"")
			if endIdx >= 0 {
				// Found the closing quote — finalise the multi-line value.
				multiLineBuf.WriteString("\n")
				multiLineBuf.WriteString(line[:endIdx])
				multiLineEntry.Value = processDoubleQuotedEscapes(multiLineBuf.String())
				multiLineEntry.RawValue = multiLineBuf.String()

				// An inline comment may follow the closing quote on the same line.
				rest := strings.TrimSpace(line[endIdx+1:])
				if strings.HasPrefix(rest, "#") {
					multiLineEntry.Comment = strings.TrimSpace(rest[1:])
				}

				entries = append(entries, *multiLineEntry)
				multiLineEntry = nil
				multiLineBuf.Reset()
			} else {
				// No closing quote on this line; accumulate and keep going.
				multiLineBuf.WriteString("\n")
				multiLineBuf.WriteString(line)
			}
			continue
		}

		// Blank line — preserve it for faithful round-tripping.
		if strings.TrimSpace(line) == "" {
			entries = append(entries, EnvEntry{Line: lineNum, IsBlank: true})
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Full-line comment. Store the raw comment text (with leading space if
		// present) so that WriteEnvFile can reproduce "# note" vs "#note".
		if strings.HasPrefix(trimmed, "#") {
			entries = append(entries, EnvEntry{
				Line:      lineNum,
				IsComment: true,
				Comment:   strings.TrimPrefix(trimmed, "#"),
			})
			continue
		}

		entry := EnvEntry{Line: lineNum}

		// Strip the shell "export" prefix that some generators emit.
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimPrefix(trimmed, "export ")
			entry.HasExport = true
		}

		// Locate the first "=" to separate key from value.
		// If there is no "=", treat the entire token as a bare key declaration.
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			entry.Key = strings.TrimSpace(trimmed)
			entries = append(entries, entry)
			continue
		}

		entry.Key = strings.TrimSpace(trimmed[:eqIdx])
		entry.HasValue = true
		rawValue := trimmed[eqIdx+1:]

		// Delegate to parseValueFull which handles all three quoting styles
		// and detects the start of an unterminated multi-line value.
		val, comment, isMultiLine := parseValueFull(rawValue)
		if isMultiLine {
			// Begin accumulating the multi-line value; continue on the next iteration.
			multiLineEntry = &entry
			multiLineBuf.WriteString(val)
			continue
		}

		entry.Value = val
		entry.RawValue = val
		entry.Comment = comment
		entries = append(entries, entry)
	}

	// If a multi-line value was never closed, emit it as-is (best effort).
	// This handles truncated or hand-edited files gracefully.
	if multiLineEntry != nil {
		multiLineEntry.Value = processDoubleQuotedEscapes(multiLineBuf.String())
		multiLineEntry.RawValue = multiLineBuf.String()
		entries = append(entries, *multiLineEntry)
	}

	return entries, scanner.Err()
}

// parseValueFull extracts the parsed value, any inline comment, and whether
// the double-quoted value is unterminated (i.e. continues on subsequent lines).
//
// The three quoting modes behave differently:
//   - Double-quoted: backslash escapes are processed; the value may span lines.
//   - Single-quoted: the value is literal — no escapes, no interpolation, no
//     multi-line. A "#" inside single quotes is not treated as a comment.
//   - Unquoted: the value ends at the first " #" sequence (space-hash), which
//     is the conventional inline-comment delimiter for unquoted values.
func parseValueFull(raw string) (value, comment string, isMultiLine bool) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return "", "", false
	}

	// Double-quoted value — supports escape sequences and multi-line spans.
	if strings.HasPrefix(raw, "\"") {
		content := raw[1:] // strip the opening quote
		endIdx := findClosingQuote(content)
		if endIdx < 0 {
			// The closing quote is missing; this is the start of a multi-line
			// value. Return the accumulated content so far for the caller to
			// buffer, and signal continuation with isMultiLine=true.
			return content, "", true
		}
		innerRaw := content[:endIdx]
		value = processDoubleQuotedEscapes(innerRaw)

		// Anything after the closing quote on the same line may be a comment.
		rest := strings.TrimSpace(content[endIdx+1:])
		if strings.HasPrefix(rest, "#") {
			comment = strings.TrimSpace(rest[1:])
		}
		return value, comment, false
	}

	// Single-quoted value — strictly literal; the closing "'" ends the value.
	// No escape sequences are recognised inside single quotes, matching bash
	// behaviour where $VAR and \n are both treated as plain characters.
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
		// Unterminated single quote — treat everything after the opening
		// quote as the literal value rather than failing.
		return raw[1:], "", false
	}

	// Unquoted value — the de-facto standard uses " #" (space then hash) as
	// the inline-comment delimiter. A lone "#" without a preceding space is
	// part of the value (e.g. colour codes like "#FF0000").
	commentIdx := strings.Index(raw, " #")
	if commentIdx >= 0 {
		value = strings.TrimSpace(raw[:commentIdx])
		comment = strings.TrimSpace(raw[commentIdx+2:])
		return value, comment, false
	}

	return strings.TrimSpace(raw), "", false
}

// findClosingQuote returns the index of the first unescaped double-quote
// character within s, or -1 if none is found. A backslash preceding a quote
// causes it to be skipped, matching the escape semantics for double-quoted
// .env values.
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

// processDoubleQuotedEscapes expands backslash escape sequences inside a
// double-quoted value. The set of recognised sequences mirrors what common
// .env loaders (e.g. godotenv, dotenv for Node.js) accept:
//
//	\n  → newline
//	\t  → horizontal tab
//	\r  → carriage return
//	\\  → literal backslash
//	\"  → literal double-quote
//	\$  → literal dollar sign (prevents variable interpolation)
//
// Unknown escape sequences (e.g. "\x") are passed through unchanged — the
// backslash and the following character are both emitted — so that values
// like Windows paths do not silently lose characters.
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
				// Unknown escape — preserve both the backslash and the character
				// so the value is not silently corrupted.
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
		// A trailing backslash with no following character is emitted as-is.
		b.WriteByte('\\')
	}
	return b.String()
}

// KeysNeeded returns the variable names from entries that require a value
// to be resolved from the vault. Comment-only and blank entries are excluded.
// Entries with no "=" sign (HasValue=false) are still included because the
// caller may need to supply a value even for bare key declarations.
func KeysNeeded(entries []EnvEntry) []string {
	var keys []string
	for _, e := range entries {
		if e.Key != "" && !e.IsComment && !e.IsBlank {
			keys = append(keys, e.Key)
		}
	}
	return keys
}
