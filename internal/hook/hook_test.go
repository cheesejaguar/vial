package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallAndUninstall verifies the full install → check → uninstall lifecycle
// on a fresh fake git repository with no pre-existing hook.
func TestInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()

	// Create a fake .git directory so Install does not reject the path.
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

	// Verify it is an executable shell script wrapped in the vial markers
	if !strings.HasPrefix(content, "#!/") {
		t.Errorf("hook should start with a shebang, got %q", content[:min(len(content), 20)])
	}
	if !strings.Contains(content, hookMarkerStart) || !strings.Contains(content, hookMarkerEnd) {
		t.Error("hook file missing markers")
	}
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("hook mode = %o, want owner-executable", info.Mode().Perm())
	}

	// Install again should fail — double-install is a user error.
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

	// Hook file should be removed because the vial block was the only content.
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("hook file should be removed when only vial content")
	}
}

// TestInstallAppendsToExisting verifies that installing into a repo that already
// has a pre-commit hook preserves the existing script and that uninstalling only
// removes the vial block, leaving the original content intact.
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
	if !strings.HasPrefix(content, existingContent) {
		t.Error("existing content was lost or moved")
	}
	if !strings.Contains(content, hookMarkerStart) {
		t.Error("vial hook not appended")
	}

	// Uninstall should preserve existing content and remove only the vial block.
	if err := Uninstall(dir); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	data, err = os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "existing hook") {
		t.Error("existing content was lost after uninstall")
	}
	if strings.Contains(string(data), hookMarkerStart) {
		t.Error("vial markers still present after uninstall")
	}
}

// TestInstallNoGitDir confirms that Install returns an error when the target
// directory is not a git repository.
func TestInstallNoGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := Install(dir); err == nil {
		t.Error("expected error when no .git directory")
	}
}

// TestLoadIgnorePatterns verifies that blank lines and comment lines are excluded
// and that valid patterns are returned in order.
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

// TestShouldIgnore checks the ignore logic against file-path and line-content patterns.
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
