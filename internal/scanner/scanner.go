package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvVarRef represents a reference to an environment variable found in source code.
type EnvVarRef struct {
	Name     string
	File     string
	Line     int
	Language string
}

// ScanResult holds all env var references found in a project.
type ScanResult struct {
	Refs    []EnvVarRef
	Files   int
	Scanned int
}

// langPattern defines a language-specific env var access pattern.
type langPattern struct {
	Language   string
	Extensions []string
	Patterns   []*regexp.Regexp
}

var languages = []langPattern{
	{
		Language:   "javascript",
		Extensions: []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]+)`),
			regexp.MustCompile(`process\.env\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			regexp.MustCompile(`import\.meta\.env\.([A-Z][A-Z0-9_]+)`),
			regexp.MustCompile(`Deno\.env\.get\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "python",
		Extensions: []string{".py"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`os\.environ\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			regexp.MustCompile(`os\.environ\.get\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
			regexp.MustCompile(`os\.getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "go",
		Extensions: []string{".go"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`os\.Getenv\("([A-Z][A-Z0-9_]+)"\)`),
			regexp.MustCompile(`os\.LookupEnv\("([A-Z][A-Z0-9_]+)"\)`),
		},
	},
	{
		Language:   "ruby",
		Extensions: []string{".rb"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`ENV\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			regexp.MustCompile(`ENV\.fetch\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "rust",
		Extensions: []string{".rs"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`env::var\("([A-Z][A-Z0-9_]+)"\)`),
			regexp.MustCompile(`env::var_os\("([A-Z][A-Z0-9_]+)"\)`),
		},
	},
	{
		Language:   "php",
		Extensions: []string{".php"},
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
			regexp.MustCompile(`\$_ENV\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			regexp.MustCompile(`env\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
}

// skipDirs are directories to skip during scanning.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true,
	"dist": true, "build": true, ".next": true, "__pycache__": true,
	".venv": true, "venv": true, "target": true, ".cache": true,
}

// ScanDir scans a project directory for environment variable references in source code.
func ScanDir(dir string) (*ScanResult, error) {
	result := &ScanResult{}

	// Build extension→language lookup
	extMap := make(map[string]*langPattern)
	for i := range languages {
		for _, ext := range languages[i].Extensions {
			extMap[ext] = &languages[i]
		}
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		lang, ok := extMap[ext]
		if !ok {
			return nil
		}

		result.Files++

		refs, err := scanFile(path, lang)
		if err != nil {
			return nil // skip individual file errors
		}

		result.Refs = append(result.Refs, refs...)
		result.Scanned++
		return nil
	})

	return result, err
}

// scanFile scans a single file for env var references.
func scanFile(path string, lang *langPattern) ([]EnvVarRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var refs []EnvVarRef
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, pat := range lang.Patterns {
			matches := pat.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					refs = append(refs, EnvVarRef{
						Name:     match[1],
						File:     path,
						Line:     lineNum,
						Language: lang.Language,
					})
				}
			}
		}
	}

	return refs, scanner.Err()
}

// UniqueVarNames returns deduplicated variable names from scan results.
func (r *ScanResult) UniqueVarNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, ref := range r.Refs {
		if !seen[ref.Name] {
			seen[ref.Name] = true
			names = append(names, ref.Name)
		}
	}
	return names
}

// Summary returns a human-readable summary of the scan.
func (r *ScanResult) Summary() string {
	names := r.UniqueVarNames()
	return fmt.Sprintf("Scanned %d files, found %d unique env vars across %d references",
		r.Scanned, len(names), len(r.Refs))
}

// GroupByLanguage groups references by language.
func (r *ScanResult) GroupByLanguage() map[string][]EnvVarRef {
	groups := make(map[string][]EnvVarRef)
	for _, ref := range r.Refs {
		groups[ref.Language] = append(groups[ref.Language], ref)
	}
	return groups
}

// FilterMissing returns var names from the scan that are NOT in the provided set.
func (r *ScanResult) FilterMissing(haveKeys map[string]bool) []string {
	var missing []string
	for _, name := range r.UniqueVarNames() {
		normalized := strings.ToUpper(name)
		if !haveKeys[normalized] {
			missing = append(missing, name)
		}
	}
	return missing
}
