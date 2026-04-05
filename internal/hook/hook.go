// Package hook manages the vial git pre-commit hook that prevents vault secrets
// from being accidentally committed to source control.
//
// Install writes a small shell fragment into a repository's .git/hooks/pre-commit
// file.  The fragment calls "vial hook check --staged" before each commit; if any
// staged file contains a plaintext secret value the commit is blocked with an
// explanatory message.
//
// Marker comments (hookMarkerStart / hookMarkerEnd) delimit the vial-managed
// block so that Install and Uninstall can operate without disturbing any other
// hook logic the developer may have added.
package hook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookMarkerStart and hookMarkerEnd are the sentinel lines that bracket the
// vial-managed section of a pre-commit hook file.  They must be unique enough
// that no other tool would generate the same strings.
const (
	hookMarkerStart = "# --- vial pre-commit hook start ---"
	hookMarkerEnd   = "# --- vial pre-commit hook end ---"

	// hookScript is the shell fragment inserted between the markers.
	// It is deliberately defensive: it only runs if vial is on PATH and
	// exits 1 (blocking the commit) only when secrets are found.
	hookScript = `# --- vial pre-commit hook start ---
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

// Install adds the vial pre-commit hook to the git repository rooted at dir.
// If a pre-commit hook file already exists its content is preserved and the
// vial script is appended.  Returns an error when the hook is already present
// or when dir is not a git repository.
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

	// If a hook file already exists, check for our markers before modifying it.
	if data, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(data), hookMarkerStart) {
			return fmt.Errorf("vial hook already installed in %s", hookPath)
		}
		// Append to the existing hook, ensuring there is a trailing newline
		// between the existing content and the vial block.
		content := string(data)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + hookScript + "\n"
		return os.WriteFile(hookPath, []byte(content), 0755)
	}

	// No existing hook — create a minimal POSIX sh script from scratch.
	content := "#!/bin/sh\n\n" + hookScript + "\n"
	return os.WriteFile(hookPath, []byte(content), 0755)
}

// Uninstall removes the vial-managed block from the repository's pre-commit hook.
// If the hook file contained only the vial block (nothing else of substance) the
// file is deleted entirely so git does not run an empty script on every commit.
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

	// Locate the start and end markers so we can excise exactly the vial block.
	startIdx := strings.Index(content, hookMarkerStart)
	endIdx := strings.Index(content, hookMarkerEnd)
	if startIdx < 0 || endIdx < 0 {
		return fmt.Errorf("malformed vial hook markers")
	}
	// Advance endIdx past the marker text itself so it is included in the removal.
	endIdx += len(hookMarkerEnd)

	// Trim surrounding blank lines to avoid leaving a double-blank gap in the
	// remaining hook content.
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

	// When nothing meaningful remains, remove the file entirely rather than
	// leaving an empty or shebang-only hook that git would execute uselessly.
	if strings.TrimSpace(remaining) == "#!/bin/sh" || strings.TrimSpace(remaining) == "" {
		return os.Remove(hookPath)
	}

	if !strings.HasSuffix(remaining, "\n") {
		remaining += "\n"
	}
	return os.WriteFile(hookPath, []byte(remaining), 0755)
}

// IsInstalled reports whether the vial pre-commit hook is present in the
// repository rooted at dir.
func IsInstalled(dir string) bool {
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), hookMarkerStart)
}

// LoadIgnorePatterns reads substring patterns from a .vialignore file in dir.
// Each non-blank, non-comment line is treated as a literal substring; if the
// pattern appears in a file path or in a matching line the finding is suppressed.
// A missing .vialignore is silently treated as an empty list.
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
