package importer

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestJSONImporterNoArgs(t *testing.T) {
	imp := &JSONImporter{}
	_, err := imp.Import(nil)
	if err == nil {
		t.Error("expected error with no args")
	}
}

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
