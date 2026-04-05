package hook

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

// Finding describes a single location where a vault secret value was detected
// in a staged file.  The secret value itself is never stored in a Finding to
// avoid accidentally printing it in error messages.
type Finding struct {
	File    string // repository-relative path of the staged file
	Line    int    // 1-based line number within the staged content
	KeyName string // vault key whose plaintext value was found on this line
}

// ScanStaged scans the git index (staged content) for plaintext occurrences of
// any value in secretValues.  It returns one Finding per match.
//
// secretValues maps vault key names to their decrypted values; the caller is
// responsible for providing only the keys relevant to the current project.
//
// ignorePatterns are substring patterns loaded from .vialignore.  A finding is
// suppressed when the pattern appears in either the file path or the matching line.
//
// Values shorter than minSecretLen characters are skipped because single-digit
// or common-word values would produce an unacceptable number of false positives.
func ScanStaged(dir string, secretValues map[string]string, ignorePatterns []string) ([]Finding, error) {
	// Get list of staged files
	stagedFiles, err := getStagedFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("getting staged files: %w", err)
	}

	if len(stagedFiles) == 0 {
		return nil, nil
	}

	// Filter out secrets that are too short to be meaningful (avoid false positives).
	// Eight characters is a heuristic floor: shorter values (e.g. "true", "1234")
	// are common in non-secret configuration and would generate noise.
	const minSecretLen = 8
	checkSecrets := make(map[string]string)
	for key, val := range secretValues {
		if len(val) >= minSecretLen {
			checkSecrets[key] = val
		}
	}

	if len(checkSecrets) == 0 {
		return nil, nil
	}

	var findings []Finding

	for _, file := range stagedFiles {
		// Read the staged (index) version of the file rather than the working-tree
		// version so the scan reflects exactly what will be committed.
		content, err := getStagedFileContent(dir, file)
		if err != nil {
			continue // skip files we can't read (e.g. deleted in working tree)
		}

		scanner := bufio.NewScanner(strings.NewReader(content))
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			for keyName, secretVal := range checkSecrets {
				if strings.Contains(line, secretVal) {
					if shouldIgnore(file, line, ignorePatterns) {
						continue
					}
					findings = append(findings, Finding{
						File:    file,
						Line:    lineNum,
						KeyName: keyName,
					})
				}
			}
		}
	}

	return findings, nil
}

// getStagedFiles returns the repository-relative paths of files that are staged
// for the next commit.  The --diff-filter=ACMR flag limits results to Added,
// Copied, Modified, and Renamed files — deleted files cannot contain leaked
// secrets and are excluded to avoid spurious errors.
func getStagedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// getStagedFileContent returns the content of file as it exists in the git index
// (i.e. the staged version) using "git show :<path>".  This guarantees that the
// scan always matches what git will actually commit, not the possibly-dirty
// working-tree copy.
func getStagedFileContent(dir, file string) (string, error) {
	cmd := exec.Command("git", "show", ":"+file)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// shouldIgnore reports whether a finding should be suppressed because either
// the file path or the matching line contains one of the ignore patterns.
// Patterns are treated as literal substrings, not globs or regexes, so they
// are fast and predictable for end-users.
func shouldIgnore(file, line string, patterns []string) bool {
	for _, pat := range patterns {
		if strings.Contains(file, pat) || strings.Contains(line, pat) {
			return true
		}
	}
	return false
}
