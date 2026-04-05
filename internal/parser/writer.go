package parser

import (
	"fmt"
	"os"
	"strings"
)

// envFilePerms are the Unix permissions applied to written .env files.
// 0600 (owner read/write only) is the conventional choice for files that
// contain plaintext secrets, preventing other users on the same system from
// reading them. This mirrors the vault file's own permission model.
const envFilePerms = 0600

// WriteEnvFile writes a populated .env file to path using parsed entries as
// the template and secrets as the resolved plaintext values.
//
// The function iterates entries in order so that comments, blank lines, and
// key ordering from the source .env.example are preserved in the output. For
// each key entry, if the key appears in secrets the secret value is written;
// otherwise the placeholder value stored in the entry is used. This means
// keys that could not be matched in the vault still appear in the output file,
// making it easy for the developer to spot gaps.
//
// Values containing whitespace, hash characters, or quotes are automatically
// double-quoted with necessary escapes applied by [quoteIfNeeded], so the
// output is always parseable by any compliant .env reader.
func WriteEnvFile(path string, entries []EnvEntry, secrets map[string]string) error {
	var lines []string

	for _, e := range entries {
		switch {
		case e.IsBlank:
			// Preserve blank lines exactly — they often serve as visual
			// section separators in hand-maintained .env.example files.
			lines = append(lines, "")

		case e.IsComment:
			// Reconstruct the comment line with its original "#" prefix.
			// The Comment field retains the text after "#" verbatim, including
			// any leading space, so "# note" round-trips as "# note".
			lines = append(lines, "#"+e.Comment)

		default:
			// Use the resolved secret value if available; fall back to whatever
			// was in the template entry (which may be a placeholder or empty).
			val := e.Value
			if resolved, ok := secrets[e.Key]; ok {
				val = resolved
			}

			line := e.Key + "=" + quoteIfNeeded(val)
			if e.Comment != "" {
				// Re-attach the inline comment with the standard " # " delimiter.
				line += " # " + e.Comment
			}
			lines = append(lines, line)
		}
	}

	// Join with Unix newlines and add a trailing newline, which is the POSIX
	// convention for text files and avoids "no newline at end of file" noise
	// in version control diffs.
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), envFilePerms); err != nil {
		return fmt.Errorf("writing .env file: %w", err)
	}

	return nil
}

// quoteIfNeeded wraps val in double quotes if it contains characters that
// would be ambiguous or mis-parsed in an unquoted context. Specifically it
// quotes values containing:
//
//   - Spaces or tabs (would be treated as a delimiter by naive parsers)
//   - Newlines (would break the single-line key=value assumption)
//   - Double or single quote characters (to avoid premature termination)
//   - Hash "#" (could be misread as the start of an inline comment)
//
// Inside the double-quoted value, backslashes and embedded double-quotes are
// escaped so the output can be parsed back to the original string. Newlines
// inside a value are encoded as the two-character escape "\n" rather than a
// literal newline, keeping the output single-line per key for maximum
// compatibility with simple line-oriented .env parsers.
//
// Empty values are returned as-is (KEY=) because quoting an empty string
// as KEY="" is redundant and adds visual noise.
func quoteIfNeeded(val string) string {
	if val == "" {
		return ""
	}
	// Characters that require the value to be wrapped in double quotes.
	if strings.ContainsAny(val, " \t\n\"'#") {
		// Escape in order: backslashes first (before any new backslashes are
		// introduced by subsequent replacements), then double-quotes, then newlines.
		escaped := strings.ReplaceAll(val, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return "\"" + escaped + "\""
	}
	return val
}
