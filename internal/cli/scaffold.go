package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/scanner"
)

// scaffoldCmd scans a project's source code for environment variable references
// and generates a .env.example file with one empty entry per discovered variable.
// Each entry is preceded by a comment listing the source files and line numbers
// where the variable is referenced, helping developers understand which part of
// the codebase requires each secret.
//
// When the output file already exists, scaffold operates in merge mode by
// default: it appends only the variables that are not already present, leaving
// existing entries (and their comments) intact. Pass --overwrite to replace the
// file entirely.
var scaffoldCmd = &cobra.Command{
	Use:   "scaffold [DIR]",
	Short: "Auto-generate .env.example from source code",
	Long: `Scan project source code for environment variable references and generate
a .env.example file. Detects env vars in JavaScript, TypeScript, Python, Go,
Ruby, Rust, and PHP.

If .env.example already exists, merges new variables without removing existing ones.

Examples:
  vial scaffold                # scan current directory
  vial scaffold ./my-project   # scan specific directory
  vial scaffold --overwrite    # replace existing .env.example`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScaffold,
}

var (
	// scaffoldOverwrite replaces the existing .env.example entirely when true.
	// Default (false) preserves existing entries and appends only new ones.
	scaffoldOverwrite bool

	// scaffoldOutput overrides the default output path (.env.example inside the
	// target directory). Useful when a project keeps its template file in a
	// non-standard location.
	scaffoldOutput string
)

func init() {
	scaffoldCmd.Flags().BoolVar(&scaffoldOverwrite, "overwrite", false, "Overwrite existing .env.example instead of merging")
	scaffoldCmd.Flags().StringVarP(&scaffoldOutput, "output", "o", "", "Output file path (default: .env.example in target dir)")
	rootCmd.AddCommand(scaffoldCmd)
}

// runScaffold implements the "vial scaffold" command. The high-level flow is:
//
//  1. Scan all source files under the target directory for env var references.
//  2. Build a deduplicated map from variable name to the list of source
//     locations (file:line) where it is used.
//  3. Sort the variable names alphabetically for deterministic output.
//  4. Either merge new names into the existing .env.example or generate a
//     fresh file, depending on the --overwrite flag and whether a file exists.
func runScaffold(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if err := loadConfig(); err != nil {
		return err
	}

	// Default output path follows cfg.EnvExample (usually ".env.example") so
	// that the generated file is co-located with the project's source code.
	outputPath := scaffoldOutput
	if outputPath == "" {
		outputPath = filepath.Join(absDir, cfg.EnvExample)
	}

	fmt.Printf("🔍 Scanning %s for env var references...\n", mutedText(absDir))
	result, err := scanner.ScanDir(absDir)
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}

	if len(result.Refs) == 0 {
		fmt.Println(mutedText("No environment variable references found in source code."))
		return nil
	}

	fmt.Printf("  %s\n", result.Summary())

	// varInfo groups a variable name with the deduplicated set of source
	// locations so each generated comment lists every usage site.
	type varInfo struct {
		name  string
		files []string // "relpath:line" entries
	}

	// Build varMap from scanner results, collapsing duplicate (name, location)
	// pairs that arise when a variable is referenced more than once per file.
	varMap := make(map[string]*varInfo)
	for _, ref := range result.Refs {
		relPath, _ := filepath.Rel(absDir, ref.File)
		if relPath == "" {
			relPath = ref.File
		}
		loc := fmt.Sprintf("%s:%d", relPath, ref.Line)
		if vi, ok := varMap[ref.Name]; ok {
			// Deduplicate location strings — the same variable can be referenced
			// on multiple lines in the same file.
			found := false
			for _, f := range vi.files {
				if f == loc {
					found = true
					break
				}
			}
			if !found {
				vi.files = append(vi.files, loc)
			}
		} else {
			varMap[ref.Name] = &varInfo{name: ref.Name, files: []string{loc}}
		}
	}

	// Sort alphabetically so the generated file has a stable order across runs,
	// making diffs readable and avoiding spurious changes in version control.
	varNames := make([]string, 0, len(varMap))
	for name := range varMap {
		varNames = append(varNames, name)
	}
	sort.Strings(varNames)

	// In merge mode, read the existing file and collect keys that are already
	// present so we can skip them when building the new content block.
	existingKeys := make(map[string]bool)
	var existingContent string
	if !scaffoldOverwrite {
		if data, err := os.ReadFile(outputPath); err == nil {
			existingContent = string(data)
			for _, line := range strings.Split(existingContent, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.Index(line, "="); idx > 0 {
					existingKeys[line[:idx]] = true
				}
			}
		}
	}

	var lines []string

	if existingContent != "" && !scaffoldOverwrite {
		// Merge mode: preserve every line of the existing file and append a
		// clearly demarcated section containing only the newly discovered vars.
		lines = append(lines, strings.TrimRight(existingContent, "\n"))

		var newVars []string
		for _, name := range varNames {
			if !existingKeys[name] {
				newVars = append(newVars, name)
			}
		}

		if len(newVars) == 0 {
			fmt.Println("  .env.example is already up to date — no new vars found.")
			return nil
		}

		lines = append(lines, "")
		lines = append(lines, "# Auto-discovered by vial scaffold")
		for _, name := range newVars {
			vi := varMap[name]
			lines = append(lines, fmt.Sprintf("# Used in: %s", strings.Join(vi.files, ", ")))
			lines = append(lines, name+"=")
		}

		fmt.Printf("  %s Added %s new variable(s) to %s\n", successIcon(), countText(fmt.Sprintf("%d", len(newVars))), filepath.Base(outputPath))
	} else {
		// Fresh generation: emit a standard file header followed by one block
		// per variable. Each block contains a provenance comment and the
		// empty KEY= assignment that tools like dotenv expect.
		lines = append(lines, "# Environment variables for "+filepath.Base(absDir))
		lines = append(lines, "# Generated by: vial scaffold")
		lines = append(lines, "# Source: scanned project source code")
		lines = append(lines, "")

		for i, name := range varNames {
			vi := varMap[name]
			lines = append(lines, fmt.Sprintf("# Used in: %s", strings.Join(vi.files, ", ")))
			lines = append(lines, name+"=")
			// Blank line between entries for readability; omit after the last one.
			if i < len(varNames)-1 {
				lines = append(lines, "")
			}
		}

		fmt.Printf("  %s Generated %s with %s variable(s)\n", successIcon(), boldText(filepath.Base(outputPath)), countText(fmt.Sprintf("%d", len(varNames))))
	}

	// Write with mode 0600 so the file is not world-readable. Developers may
	// fill in real values for local testing before remembering to .gitignore it,
	// so restrictive permissions provide a small safety margin.
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Printf("  %s %s\n", arrowIcon(), mutedText(outputPath))
	return nil
}
