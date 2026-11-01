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

// TestIsInteractive verifies that isInteractive reports false when stdin is a
// pipe (as it is for `echo pw | vial ...` or CI scripts). Stdin is swapped for
// a pipe so the result does not depend on how the test binary was launched.
func TestIsInteractive(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	if isInteractive() {
		t.Error("isInteractive() = true with stdin as a pipe, want false")
	}
}
