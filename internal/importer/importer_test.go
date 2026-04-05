package importer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestJSONImporter verifies the happy path: a valid flat JSON file is read
// and its key-value pairs are returned as Secret values.
func TestJSONImporter(t *testing.T) {
	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "secrets.json")

	content := `{"OPENAI_API_KEY": "sk-abc123", "DB_URL": "postgres://localhost/db"}`
	if err := os.WriteFile(jsonFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	imp := &JSONImporter{}
	if !imp.Available() {
		t.Error("JSON importer should always be available")
	}

	secrets, err := imp.Import([]string{jsonFile})
	if err != nil {
		t.Fatal(err)
	}

	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(secrets))
	}

	found := make(map[string]string)
	for _, s := range secrets {
		found[s.Key] = s.Value
	}

	if found["OPENAI_API_KEY"] != "sk-abc123" {
		t.Errorf("unexpected OPENAI_API_KEY: %q", found["OPENAI_API_KEY"])
	}
	if found["DB_URL"] != "postgres://localhost/db" {
		t.Errorf("unexpected DB_URL: %q", found["DB_URL"])
	}
}

// TestJSONImporterNoArgs confirms that calling Import with no file path
// returns an error rather than panicking.
func TestJSONImporterNoArgs(t *testing.T) {
	imp := &JSONImporter{}
	_, err := imp.Import(nil)
	if err == nil {
		t.Error("expected error with no args")
	}
}

// TestJSONImporterInvalidJSON confirms that a file containing non-JSON content
// produces a descriptive error, not a silent empty result.
func TestJSONImporterInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "bad.json")
	os.WriteFile(jsonFile, []byte("not json"), 0600)

	imp := &JSONImporter{}
	_, err := imp.Import([]string{jsonFile})
	if err == nil {
		t.Error("expected error with invalid JSON")
	}
}

// TestGetBackend verifies that every documented backend name resolves to a
// non-nil Backend with the correct Name(), and that an unknown name returns
// an error.
func TestGetBackend(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"json", false},
		{"1password", false},
		{"doppler", false},
		{"vercel", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		b, err := GetBackend(tt.name)
		if tt.wantErr && err == nil {
			t.Errorf("GetBackend(%q): expected error", tt.name)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("GetBackend(%q): unexpected error: %v", tt.name, err)
		}
		if !tt.wantErr && b.Name() != tt.name {
			t.Errorf("GetBackend(%q).Name() = %q", tt.name, b.Name())
		}
	}
}

// TestIsEnvVarName covers both valid and invalid env var label patterns used
// by the 1Password importer to decide which item fields to import.
func TestIsEnvVarName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"OPENAI_API_KEY", true},
		{"DB_URL", true},
		{"A", true},
		{"_PRIVATE", true},
		{"lowercase", false},
		{"has space", false},
		{"123START", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isEnvVarName(tt.input); got != tt.want {
			t.Errorf("isEnvVarName(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
