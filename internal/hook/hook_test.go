package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()

	// Create a fake .git directory
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Install
	if err := Install(dir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify hook file exists
	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}

	content := string(data)
	if got := IsInstalled(dir); !got {
		t.Error("IsInstalled returned false after install")
	}

	if got := content; got == "" {
		t.Error("hook file is empty")
	}

	// Verify it contains the markers
	if got := content; !contains(got, hookMarkerStart) || !contains(got, hookMarkerEnd) {
		t.Error("hook file missing markers")
	}

	// Install again should fail
	if err := Install(dir); err == nil {
		t.Error("expected error on double install")
	}

	// Uninstall
	if err := Uninstall(dir); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if got := IsInstalled(dir); got {
		t.Error("IsInstalled returned true after uninstall")
	}

	// Hook file should be removed (was only vial content)
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("hook file should be removed when only vial content")
	}
}

func TestInstallAppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create existing pre-commit hook
	hookPath := filepath.Join(gitDir, "pre-commit")
	existingContent := "#!/bin/sh\necho 'existing hook'\n"
	if err := os.WriteFile(hookPath, []byte(existingContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Install
	if err := Install(dir); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !contains(content, "existing hook") {
		t.Error("existing content was lost")
	}
	if !contains(content, hookMarkerStart) {
		t.Error("vial hook not appended")
	}

	// Uninstall should preserve existing content
	if err := Uninstall(dir); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	data, err = os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "existing hook") {
		t.Error("existing content was lost after uninstall")
	}
	if contains(string(data), hookMarkerStart) {
		t.Error("vial markers still present after uninstall")
	}
}

func TestInstallNoGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := Install(dir); err == nil {
		t.Error("expected error when no .git directory")
	}
}

func TestLoadIgnorePatterns(t *testing.T) {
	dir := t.TempDir()

	content := "# Comment\ntest-fixture\n\n*.min.js\n"
	if err := os.WriteFile(filepath.Join(dir, ".vialignore"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	patterns := LoadIgnorePatterns(dir)
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d: %v", len(patterns), patterns)
	}
	if patterns[0] != "test-fixture" || patterns[1] != "*.min.js" {
		t.Errorf("unexpected patterns: %v", patterns)
	}
}

func TestShouldIgnore(t *testing.T) {
	patterns := []string{"test-fixture", "mock"}

	if !shouldIgnore("test-fixture/config.js", "API_KEY=abc", patterns) {
		t.Error("should ignore file matching pattern")
	}
	if !shouldIgnore("src/app.js", "mock_api_key", patterns) {
		t.Error("should ignore line matching pattern")
	}
	if shouldIgnore("src/app.js", "real_api_key=secret", patterns) {
		t.Error("should not ignore non-matching")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
