package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "****"},
		{"short", "****"},
		{"exactly12ch", "****"},
		{"sk-proj-abc123def456", "sk-p...f456"},
		{"longer-secret-value-here", "long...here"},
	}
	for _, tt := range tests {
		got := maskValue(tt.input)
		if got != tt.want {
			t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWriteFileWithDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")

	err := writeFileWithDirs(path, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("writeFileWithDirs: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", data, "hello")
	}
}

func TestIsInteractive(t *testing.T) {
	// In test environment, stdin is not a terminal
	if isInteractive() {
		t.Skip("stdin is a terminal in this test environment")
	}
}
