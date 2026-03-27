package parser

import (
	"fmt"
	"os"
	"strings"
)

const envFilePerms = 0600

// WriteEnvFile generates a .env file from parsed entries and resolved secrets.
// It preserves comments and blank lines from the template.
// The secrets map contains key→plaintext_value for keys that were resolved.
func WriteEnvFile(path string, entries []EnvEntry, secrets map[string]string) error {
	var lines []string

	for _, e := range entries {
		switch {
		case e.IsBlank:
			lines = append(lines, "")
		case e.IsComment:
			lines = append(lines, "#"+e.Comment)
		default:
			val := e.Value
			if resolved, ok := secrets[e.Key]; ok {
				val = resolved
			}

			line := e.Key + "=" + quoteIfNeeded(val)
			if e.Comment != "" {
				line += " # " + e.Comment
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), envFilePerms); err != nil {
		return fmt.Errorf("writing .env file: %w", err)
	}

	return nil
}

// quoteIfNeeded wraps a value in double quotes if it contains special characters.
func quoteIfNeeded(val string) string {
	if val == "" {
		return ""
	}
	// Quote if value contains spaces, #, quotes, or newlines
	if strings.ContainsAny(val, " \t\n\"'#") {
		escaped := strings.ReplaceAll(val, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return "\"" + escaped + "\""
	}
	return val
}
