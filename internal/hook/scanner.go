package hook

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

// Finding represents a secret found in a staged file.
type Finding struct {
	File    string
	Line    int
	KeyName string // the vault key name whose value was found
}

// ScanStaged scans git staged files for any values that match secrets in the vault.
// secretValues maps key names to their plaintext values.
// ignorePatterns are substrings to ignore (from .vialignore).
func ScanStaged(dir string, secretValues map[string]string, ignorePatterns []string) ([]Finding, error) {
	// Get list of staged files
	stagedFiles, err := getStagedFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("getting staged files: %w", err)
	}

	if len(stagedFiles) == 0 {
		return nil, nil
	}

	// Filter out secrets that are too short to be meaningful (avoid false positives)
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
		// Get the staged content of the file
		content, err := getStagedFileContent(dir, file)
		if err != nil {
			continue // skip files we can't read
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

// getStagedFiles returns the list of files staged for commit.
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

// getStagedFileContent returns the staged (index) content of a file.
func getStagedFileContent(dir, file string) (string, error) {
	cmd := exec.Command("git", "show", ":"+file)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// shouldIgnore checks if a finding should be ignored based on .vialignore patterns.
func shouldIgnore(file, line string, patterns []string) bool {
	for _, pat := range patterns {
		if strings.Contains(file, pat) || strings.Contains(line, pat) {
			return true
		}
	}
	return false
}
