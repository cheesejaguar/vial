// Package scanner walks a project directory tree and extracts references to
// environment variables from source code files.
//
// The scanner recognises language-specific idioms for reading env vars
// (e.g. process.env.KEY in JavaScript, os.Getenv("KEY") in Go) using a set
// of per-language regular expressions. Results include the variable name,
// source file path, line number, and detected language, making it easy to
// audit which keys the codebase actually uses and to cross-reference them
// against the vault.
//
// The scanning strategy deliberately favours recall over precision: it finds
// every statically-resolvable reference but will miss dynamically-constructed
// names like process.env[computedKey]. That trade-off is intentional because
// false negatives (missing a required secret) are more harmful than false
// positives (suggesting a secret that isn't actually needed).
package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvVarRef records a single reference to an environment variable found in
// source code. Each unique (Name, File, Line) tuple represents one access site.
type EnvVarRef struct {
	Name     string // environment variable name as it appears in source (e.g. "DATABASE_URL")
	File     string // absolute or relative path to the source file
	Line     int    // 1-based line number within File
	Language string // detected language identifier (e.g. "javascript", "go")
}

// ScanResult aggregates all [EnvVarRef] values found during a directory scan
// and tracks basic scan statistics.
type ScanResult struct {
	Refs    []EnvVarRef // all env var references, including duplicates across files
	Files   int         // total number of files with a recognised extension (attempted)
	Scanned int         // files successfully read and scanned (Files minus read errors)
}

// langPattern associates a language name and its source file extensions with
// the compiled regular expressions used to detect env var access in that language.
// Each Patterns entry must capture the variable name in group 1.
type langPattern struct {
	Language   string           // human-readable language name used in EnvVarRef.Language
	Extensions []string         // file extensions including the leading dot
	Patterns   []*regexp.Regexp // one or more patterns; all are applied to each line
}

// languages is the registry of supported languages and their env-var access patterns.
// Patterns are compiled once at package init time via regexp.MustCompile to avoid
// per-scan overhead. Each pattern uses capture group 1 for the variable name and
// restricts matches to SCREAMING_SNAKE_CASE names (uppercase letter start, then
// uppercase letters, digits, and underscores) to avoid matching unrelated identifiers.
var languages = []langPattern{
	{
		Language:   "javascript",
		Extensions: []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"},
		Patterns: []*regexp.Regexp{
			// Node.js / Bun: process.env.KEY (dot access)
			regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]+)`),
			// Node.js / Bun: process.env["KEY"] (bracket access)
			regexp.MustCompile(`process\.env\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			// Vite / import.meta.env (used in browser builds with Vite/Rollup)
			regexp.MustCompile(`import\.meta\.env\.([A-Z][A-Z0-9_]+)`),
			// Deno runtime
			regexp.MustCompile(`Deno\.env\.get\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "python",
		Extensions: []string{".py"},
		Patterns: []*regexp.Regexp{
			// os.environ["KEY"] (raises KeyError if missing)
			regexp.MustCompile(`os\.environ\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			// os.environ.get("KEY") (returns None if missing)
			regexp.MustCompile(`os\.environ\.get\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
			// os.getenv("KEY") (functional alias for os.environ.get)
			regexp.MustCompile(`os\.getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "go",
		Extensions: []string{".go"},
		Patterns: []*regexp.Regexp{
			// os.Getenv("KEY") — returns empty string if unset
			regexp.MustCompile(`os\.Getenv\("([A-Z][A-Z0-9_]+)"\)`),
			// os.LookupEnv("KEY") — distinguishes unset from empty
			regexp.MustCompile(`os\.LookupEnv\("([A-Z][A-Z0-9_]+)"\)`),
		},
	},
	{
		Language:   "ruby",
		Extensions: []string{".rb"},
		Patterns: []*regexp.Regexp{
			// ENV["KEY"] — returns nil if unset
			regexp.MustCompile(`ENV\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			// ENV.fetch("KEY") — raises KeyError if unset (unless a default is given)
			regexp.MustCompile(`ENV\.fetch\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
	{
		Language:   "rust",
		Extensions: []string{".rs"},
		Patterns: []*regexp.Regexp{
			// std::env::var("KEY") — returns Result<String>
			regexp.MustCompile(`env::var\("([A-Z][A-Z0-9_]+)"\)`),
			// std::env::var_os("KEY") — returns Option<OsString>
			regexp.MustCompile(`env::var_os\("([A-Z][A-Z0-9_]+)"\)`),
		},
	},
	{
		Language:   "php",
		Extensions: []string{".php"},
		Patterns: []*regexp.Regexp{
			// getenv("KEY") — C-style function
			regexp.MustCompile(`getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
			// $_ENV["KEY"] — superglobal array
			regexp.MustCompile(`\$_ENV\[['"]([A-Z][A-Z0-9_]+)['"]\]`),
			// env("KEY") — Laravel / Symfony helper function
			regexp.MustCompile(`env\(['"]([A-Z][A-Z0-9_]+)['"]\)`),
		},
	},
}

// skipDirs is the set of directory names that should be skipped entirely
// during the walk. These are directories that contain generated artifacts,
// third-party dependencies, or VCS metadata — none of which should be treated
// as application source code.
var skipDirs = map[string]bool{
	"node_modules": true, // npm/yarn/pnpm dependency tree
	".git":         true, // git object store
	"vendor":       true, // Go/PHP vendored dependencies
	"dist":         true, // compiled output (webpack, tsc, etc.)
	"build":        true, // compiled output (various build systems)
	".next":        true, // Next.js build cache and output
	"__pycache__":  true, // Python bytecode cache
	".venv":        true, // Python virtual environment (hidden)
	"venv":         true, // Python virtual environment (visible)
	"target":       true, // Rust/Maven compiled output
	".cache":       true, // generic cache directory
}

// ScanDir recursively walks dir, scanning every source file with a recognised
// extension for environment variable references. Directories in [skipDirs] are
// pruned from the walk to avoid scanning generated code and dependencies.
//
// Individual file read errors are silently skipped so that one unreadable file
// (e.g. a binary with a .go extension, or a symlink to a missing target) does
// not abort the entire scan.
func ScanDir(dir string) (*ScanResult, error) {
	result := &ScanResult{}

	// Build a flat extension→langPattern map for O(1) lookup during the walk.
	// We index into the languages slice rather than copying to avoid duplicating
	// the compiled regexp values.
	extMap := make(map[string]*langPattern)
	for i := range languages {
		for _, ext := range languages[i].Extensions {
			extMap[ext] = &languages[i]
		}
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Propagating walk errors for a single entry would be too aggressive;
			// skip the entry and continue so the rest of the tree is still scanned.
			return nil
		}

		if info.IsDir() {
			if skipDirs[info.Name()] {
				// filepath.SkipDir tells Walk not to descend into this directory.
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		lang, ok := extMap[ext]
		if !ok {
			// File extension not associated with any supported language; skip it.
			return nil
		}

		result.Files++ // count every recognised-extension file attempted

		refs, err := scanFile(path, lang)
		if err != nil {
			// Skip files that cannot be read (permissions, broken symlinks, etc.).
			return nil
		}

		result.Refs = append(result.Refs, refs...)
		result.Scanned++ // count only files that were successfully read
		return nil
	})

	return result, err
}

// scanFile reads path line by line and applies all patterns from lang to each
// line. All matching variable names on a line are captured and returned as
// [EnvVarRef] values with their line number set. Using a line-oriented scan
// means very large source files do not need to be loaded into memory at once.
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

		// Apply each pattern in turn. Multiple patterns may match the same line
		// (e.g. two different access forms on the same line), and a single pattern
		// may produce multiple matches (FindAllStringSubmatch returns all of them).
		for _, pat := range lang.Patterns {
			matches := pat.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					refs = append(refs, EnvVarRef{
						Name:     match[1], // capture group 1 holds the variable name
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

// UniqueVarNames returns the deduplicated set of variable names found across
// all references, preserving the order in which each name was first seen.
// This is typically used to build the list of vault keys to look up.
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

// Summary returns a single human-readable sentence describing the scan outcome,
// suitable for display in CLI output.
func (r *ScanResult) Summary() string {
	names := r.UniqueVarNames()
	return fmt.Sprintf("Scanned %d files, found %d unique env vars across %d references",
		r.Scanned, len(names), len(r.Refs))
}

// GroupByLanguage partitions all references by their Language field. The
// returned map keys are language identifiers as recorded in [langPattern.Language].
// This is used to produce per-language breakdowns in the CLI's scan output.
func (r *ScanResult) GroupByLanguage() map[string][]EnvVarRef {
	groups := make(map[string][]EnvVarRef)
	for _, ref := range r.Refs {
		groups[ref.Language] = append(groups[ref.Language], ref)
	}
	return groups
}

// FilterMissing returns the names of env vars found in the scan that are NOT
// present in haveKeys. The comparison is case-insensitive on the scan side:
// names are normalised to uppercase before the lookup, so a source file that
// accidentally uses a lowercase name (e.g. a non-standard variable) is still
// correctly compared against the uppercase vault keys.
//
// The haveKeys map is expected to use uppercase keys, consistent with the vault
// storage format. This method is used by the `brew` (run) command to warn the
// user about referenced variables that have not yet been added to the vault.
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
