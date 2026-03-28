package hook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	hookMarkerStart = "# --- vial pre-commit hook start ---"
	hookMarkerEnd   = "# --- vial pre-commit hook end ---"
	hookScript      = `# --- vial pre-commit hook start ---
# Installed by: vial hook install
# Scans staged files for leaked vault secrets before commit.
if command -v vial >/dev/null 2>&1; then
  vial hook check --staged
  if [ $? -ne 0 ]; then
    echo ""
    echo "Commit blocked by vial: secrets detected in staged files."
    echo "Use 'git commit --no-verify' to bypass (not recommended)."
    exit 1
  fi
fi
# --- vial pre-commit hook end ---`
)

// Install adds the vial pre-commit hook to the git repository at dir.
// If a pre-commit hook already exists, it appends the vial section.
func Install(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", dir)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Check if already installed
	if data, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(data), hookMarkerStart) {
			return fmt.Errorf("vial hook already installed in %s", hookPath)
		}
		// Append to existing hook
		content := string(data)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + hookScript + "\n"
		return os.WriteFile(hookPath, []byte(content), 0755)
	}

	// Create new hook file
	content := "#!/bin/sh\n\n" + hookScript + "\n"
	return os.WriteFile(hookPath, []byte(content), 0755)
}

// Uninstall removes the vial pre-commit hook section from the git repository.
func Uninstall(dir string) error {
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")

	data, err := os.ReadFile(hookPath)
	if err != nil {
		return fmt.Errorf("no pre-commit hook found")
	}

	content := string(data)
	if !strings.Contains(content, hookMarkerStart) {
		return fmt.Errorf("vial hook not found in pre-commit")
	}

	// Remove the vial section
	startIdx := strings.Index(content, hookMarkerStart)
	endIdx := strings.Index(content, hookMarkerEnd)
	if startIdx < 0 || endIdx < 0 {
		return fmt.Errorf("malformed vial hook markers")
	}
	endIdx += len(hookMarkerEnd)

	// Remove the section and any surrounding blank lines
	before := content[:startIdx]
	after := content[endIdx:]
	before = strings.TrimRight(before, "\n")
	after = strings.TrimLeft(after, "\n")

	remaining := before
	if after != "" {
		if remaining != "" {
			remaining += "\n\n"
		}
		remaining += after
	}

	if strings.TrimSpace(remaining) == "#!/bin/sh" || strings.TrimSpace(remaining) == "" {
		// Hook file is now empty, remove it
		return os.Remove(hookPath)
	}

	if !strings.HasSuffix(remaining, "\n") {
		remaining += "\n"
	}
	return os.WriteFile(hookPath, []byte(remaining), 0755)
}

// IsInstalled checks if the vial hook is installed in the given directory.
func IsInstalled(dir string) bool {
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), hookMarkerStart)
}

// LoadIgnorePatterns loads patterns from .vialignore file.
// Each line is a substring pattern; lines starting with # are comments.
func LoadIgnorePatterns(dir string) []string {
	f, err := os.Open(filepath.Join(dir, ".vialignore"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}
