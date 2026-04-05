package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseBasic covers the common single-line cases that every real-world
// .env file is likely to contain: plain values, empty values, bare keys,
// the export prefix, full-line comments, blank lines, inline comments,
// values with extra "=" characters, and both quoting styles.
func TestParseBasic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []EnvEntry
	}{
		{
			name:  "simple key=value",
			input: "API_KEY=abc123",
			want: []EnvEntry{
				{Key: "API_KEY", Value: "abc123", HasValue: true, Line: 1},
			},
		},
		{
			name:  "empty value",
			input: "API_KEY=",
			want: []EnvEntry{
				{Key: "API_KEY", Value: "", HasValue: true, Line: 1},
			},
		},
		{
			name:  "key only (no equals)",
			input: "API_KEY",
			want: []EnvEntry{
				{Key: "API_KEY", Line: 1},
			},
		},
		{
			name:  "export prefix",
			input: "export API_KEY=abc123",
			want: []EnvEntry{
				{Key: "API_KEY", Value: "abc123", HasValue: true, HasExport: true, Line: 1},
			},
		},
		{
			name:  "full line comment",
			input: "# This is a comment",
			want: []EnvEntry{
				{IsComment: true, Comment: " This is a comment", Line: 1},
			},
		},
		{
			name:  "blank line",
			input: "   ",
			want: []EnvEntry{
				{IsBlank: true, Line: 1},
			},
		},
		{
			name:  "inline comment",
			input: "API_KEY=abc123 # Your API key",
			want: []EnvEntry{
				{Key: "API_KEY", Value: "abc123", HasValue: true, Comment: "Your API key", Line: 1},
			},
		},
		{
			name:  "value with equals sign",
			input: "URL=postgres://host/db?opt=val",
			want: []EnvEntry{
				{Key: "URL", Value: "postgres://host/db?opt=val", HasValue: true, Line: 1},
			},
		},
		{
			name:  "double quoted value",
			input: `API_KEY="abc 123"`,
			want: []EnvEntry{
				{Key: "API_KEY", Value: "abc 123", HasValue: true, Line: 1},
			},
		},
		{
			name:  "single quoted value",
			input: `API_KEY='abc 123'`,
			want: []EnvEntry{
				{Key: "API_KEY", Value: "abc 123", HasValue: true, Line: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if len(entries) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(entries), len(tt.want))
			}

			for i, got := range entries {
				want := tt.want[i]
				if got.Key != want.Key {
					t.Errorf("[%d] Key = %q, want %q", i, got.Key, want.Key)
				}
				if got.Value != want.Value {
					t.Errorf("[%d] Value = %q, want %q", i, got.Value, want.Value)
				}
				if got.HasValue != want.HasValue {
					t.Errorf("[%d] HasValue = %v, want %v", i, got.HasValue, want.HasValue)
				}
				if got.IsComment != want.IsComment {
					t.Errorf("[%d] IsComment = %v, want %v", i, got.IsComment, want.IsComment)
				}
				if got.IsBlank != want.IsBlank {
					t.Errorf("[%d] IsBlank = %v, want %v", i, got.IsBlank, want.IsBlank)
				}
				if got.HasExport != want.HasExport {
					t.Errorf("[%d] HasExport = %v, want %v", i, got.HasExport, want.HasExport)
				}
				if got.Comment != want.Comment {
					t.Errorf("[%d] Comment = %q, want %q", i, got.Comment, want.Comment)
				}
			}
		})
	}
}

// TestParseDoubleQuotedEscapes verifies that all recognised backslash escape
// sequences inside double-quoted values are expanded to their intended bytes,
// and that unknown sequences are preserved unchanged rather than silently
// dropping the backslash.
func TestParseDoubleQuotedEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		value string
	}{
		{"newline escape", `KEY="hello\nworld"`, "hello\nworld"},
		{"tab escape", `KEY="hello\tworld"`, "hello\tworld"},
		{"carriage return", `KEY="hello\rworld"`, "hello\rworld"},
		{"escaped backslash", `KEY="path\\to\\file"`, "path\\to\\file"},
		{"escaped quote", `KEY="say \"hello\""`, `say "hello"`},
		{"escaped dollar", `KEY="price is \$100"`, "price is $100"},
		{"unknown escape kept", `KEY="hello\xworld"`, `hello\xworld`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("got %d entries, want 1", len(entries))
			}
			if entries[0].Value != tt.value {
				t.Errorf("Value = %q, want %q", entries[0].Value, tt.value)
			}
		})
	}
}

// TestParseMultiLineDoubleQuoted verifies that a double-quoted value whose
// closing quote appears on a later line is accumulated across lines and
// presented as a single value containing embedded newlines.
func TestParseMultiLineDoubleQuoted(t *testing.T) {
	input := "KEY=\"line1\nline2\nline3\""
	entries, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	want := "line1\nline2\nline3"
	if entries[0].Value != want {
		t.Errorf("Value = %q, want %q", entries[0].Value, want)
	}
}

// TestParseSingleQuotedLiteral verifies that escape sequences inside
// single-quoted values are NOT processed — the backslash-n is two characters,
// not a newline. This matches POSIX single-quote semantics.
func TestParseSingleQuotedLiteral(t *testing.T) {
	// Single-quoted values should NOT process escapes
	input := `KEY='hello\nworld'`
	entries, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if entries[0].Value != `hello\nworld` {
		t.Errorf("Value = %q, want %q", entries[0].Value, `hello\nworld`)
	}
}

// TestParseQuotedWithInlineComment ensures that an inline comment following a
// closing double-quote is captured in Comment and excluded from Value.
func TestParseQuotedWithInlineComment(t *testing.T) {
	input := `KEY="value" # this is a comment`
	entries, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if entries[0].Value != "value" {
		t.Errorf("Value = %q, want %q", entries[0].Value, "value")
	}
	if entries[0].Comment != "this is a comment" {
		t.Errorf("Comment = %q, want %q", entries[0].Comment, "this is a comment")
	}
}

// TestParseMultiLine exercises a realistic .env.example file that uses a
// mix of comments, blank lines, export-prefixed keys, inline comments, and
// empty values. It verifies that the total entry count, specific key values,
// the export flag, and inline comments are all parsed correctly.
func TestParseMultiLine(t *testing.T) {
	input := `# Database config
DATABASE_URL=postgresql://localhost/mydb
DB_POOL_SIZE=10

# API Keys
export OPENAI_API_KEY=sk-test-123
STRIPE_KEY=sk_test_456 # Stripe secret key

# Not configured yet
MAPBOX_TOKEN=
`
	entries, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(entries) != 10 {
		t.Fatalf("got %d entries, want 10", len(entries))
	}

	if entries[1].Key != "DATABASE_URL" || entries[1].Value != "postgresql://localhost/mydb" {
		t.Errorf("entry 1: key=%q val=%q", entries[1].Key, entries[1].Value)
	}
	if entries[5].Key != "OPENAI_API_KEY" || !entries[5].HasExport {
		t.Errorf("entry 5: key=%q export=%v", entries[5].Key, entries[5].HasExport)
	}
	if entries[6].Comment != "Stripe secret key" {
		t.Errorf("entry 6: comment=%q", entries[6].Comment)
	}
}

// TestKeysNeeded verifies that KeysNeeded returns only the keys that have an
// actual variable name, excluding comment and blank entries. It also confirms
// that bare keys (no "=") are included because the vault still needs to supply
// a value for them.
func TestKeysNeeded(t *testing.T) {
	input := `# Comment
API_KEY=abc
DB_URL=

EMPTY_KEY
`
	entries, _ := Parse(strings.NewReader(input))
	keys := KeysNeeded(entries)

	if len(keys) != 3 {
		t.Fatalf("got %d keys, want 3", len(keys))
	}

	expected := []string{"API_KEY", "DB_URL", "EMPTY_KEY"}
	for i, want := range expected {
		if keys[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want)
		}
	}
}

// TestInterpolate covers the four supported reference forms ($VAR, ${VAR},
// ${VAR:-default}, ${VAR-default}) plus edge cases like a trailing dollar sign,
// a missing variable, and the distinction between the ":-" and "-" defaults.
func TestInterpolate(t *testing.T) {
	lookup := func(key string) (string, bool) {
		m := map[string]string{
			"HOST": "localhost",
			"PORT": "5432",
			"DB":   "mydb",
		}
		v, ok := m[key]
		return v, ok
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple ${}", "postgresql://${HOST}:${PORT}/${DB}", "postgresql://localhost:5432/mydb"},
		{"simple $VAR", "host=$HOST", "host=localhost"},
		{"missing var", "val=${MISSING}", "val="},
		{"default :-", "${MISSING:-fallback}", "fallback"},
		{"default :- with set var", "${HOST:-fallback}", "localhost"},
		{"default :- with empty var", "${EMPTY_VAR:-fallback}", "fallback"},
		{"default -", "${MISSING-fallback}", "fallback"},
		{"no interpolation", "plain value", "plain value"},
		{"dollar at end", "price$", "price$"},
		{"literal dollar sign", "$", "$"},
		{"mixed", "${HOST}:${PORT}/${MISSING:-default_db}", "localhost:5432/default_db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Interpolate(tt.input, lookup)
			if got != tt.want {
				t.Errorf("Interpolate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestWriteEnvFile verifies that WriteEnvFile produces a file that preserves
// comments and blank lines from the entry list, substitutes resolved secret
// values for template placeholders, and writes the file with 0600 permissions.
func TestWriteEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	entries := []EnvEntry{
		{IsComment: true, Comment: " Database config"},
		{Key: "DB_URL", HasValue: true, Value: "placeholder"},
		{IsBlank: true},
		{Key: "API_KEY", HasValue: true, Value: ""},
	}

	secrets := map[string]string{
		"DB_URL":  "postgres://localhost/db",
		"API_KEY": "sk-abc123",
	}

	if err := WriteEnvFile(path, entries, secrets); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Database config") {
		t.Error("missing comment")
	}
	if !strings.Contains(content, "DB_URL=postgres://localhost/db") {
		t.Error("missing DB_URL")
	}
	if !strings.Contains(content, "API_KEY=sk-abc123") {
		t.Error("missing API_KEY")
	}

	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

// TestQuoteIfNeeded verifies that values requiring quoting are wrapped in
// double quotes with internal special characters properly escaped, while
// simple alphanumeric values and empty strings are left unquoted.
func TestQuoteIfNeeded(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"", ""},
		{"has space", `"has space"`},
		{"has#hash", `"has#hash"`},
		{`has"quote`, `"has\"quote"`},
	}

	for _, tt := range tests {
		got := quoteIfNeeded(tt.input)
		if got != tt.want {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
