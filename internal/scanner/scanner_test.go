package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanDirJavaScript verifies that the three JavaScript env-var access
// idioms (process.env.KEY, process.env["KEY"], and import.meta.env.KEY) are
// all detected when the file has a .js extension.
func TestScanDirJavaScript(t *testing.T) {
	dir := t.TempDir()

	// Create a JS file with env var references
	js := `
const apiKey = process.env.OPENAI_API_KEY;
const dbUrl = process.env["DATABASE_URL"];
const port = process.env.PORT;
const base = import.meta.env.VITE_BASE_URL;
`
	writeTestFile(t, dir, "app.js", js)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	names := result.UniqueVarNames()
	wantNames := map[string]bool{
		"OPENAI_API_KEY": true,
		"DATABASE_URL":   true,
		"PORT":           true,
		"VITE_BASE_URL":  true,
	}

	for _, name := range names {
		if !wantNames[name] {
			t.Errorf("unexpected var: %s", name)
		}
		delete(wantNames, name)
	}

	for name := range wantNames {
		t.Errorf("missing var: %s", name)
	}
}

// TestScanDirPython verifies that all three Python env-var access forms
// (os.environ["KEY"], os.environ.get("KEY"), os.getenv("KEY")) are detected.
func TestScanDirPython(t *testing.T) {
	dir := t.TempDir()

	py := `
import os
api_key = os.environ["OPENAI_API_KEY"]
db_url = os.environ.get("DATABASE_URL")
port = os.getenv("PORT")
`
	writeTestFile(t, dir, "app.py", py)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	assertNames(t, result.UniqueVarNames(), "OPENAI_API_KEY", "DATABASE_URL", "PORT")
}

// TestScanDirGo verifies that both os.Getenv and os.LookupEnv calls are
// captured in .go source files.
func TestScanDirGo(t *testing.T) {
	dir := t.TempDir()

	goCode := `package main

import "os"

func main() {
	key := os.Getenv("API_KEY")
	_, ok := os.LookupEnv("DEBUG")
	_ = key
	_ = ok
}
`
	writeTestFile(t, dir, "main.go", goCode)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	assertNames(t, result.UniqueVarNames(), "API_KEY", "DEBUG")
}

// TestScanDirRuby verifies that ENV["KEY"] and ENV.fetch("KEY") are both
// detected in .rb files.
func TestScanDirRuby(t *testing.T) {
	dir := t.TempDir()

	rb := `
api_key = ENV["OPENAI_API_KEY"]
db = ENV.fetch("DATABASE_URL")
`
	writeTestFile(t, dir, "app.rb", rb)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	assertNames(t, result.UniqueVarNames(), "OPENAI_API_KEY", "DATABASE_URL")
}

// TestScanDirSkipsNodeModules ensures that the node_modules directory is
// pruned from the walk and its contents are never scanned, while files in
// sibling directories (e.g. src/) are still picked up correctly.
func TestScanDirSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Create a file in node_modules that should be skipped
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nmDir, 0755)
	writeTestFile(t, nmDir, "index.js", `const x = process.env.SHOULD_BE_SKIPPED;`)

	// Create a file in src that should be scanned
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	writeTestFile(t, srcDir, "app.js", `const x = process.env.SHOULD_BE_FOUND;`)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	names := result.UniqueVarNames()
	for _, n := range names {
		if n == "SHOULD_BE_SKIPPED" {
			t.Error("node_modules should be skipped")
		}
	}

	found := false
	for _, n := range names {
		if n == "SHOULD_BE_FOUND" {
			found = true
		}
	}
	if !found {
		t.Error("src/app.js should be scanned")
	}
}

// TestScanResultFilterMissing verifies that FilterMissing correctly identifies
// variables present in the scan but absent from the provided vault key set.
func TestScanResultFilterMissing(t *testing.T) {
	result := &ScanResult{
		Refs: []EnvVarRef{
			{Name: "OPENAI_API_KEY"},
			{Name: "DATABASE_URL"},
			{Name: "PORT"},
		},
	}

	have := map[string]bool{
		"OPENAI_API_KEY": true,
	}

	assertNames(t, result.FilterMissing(have), "DATABASE_URL", "PORT")
}

// TestScanDirMultiLanguage verifies that a directory containing source files
// from multiple languages is scanned correctly and results are grouped into
// per-language buckets.
func TestScanDirMultiLanguage(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "app.js", `const x = process.env.JS_KEY;`)
	writeTestFile(t, dir, "app.py", `x = os.getenv("PY_KEY")`)
	writeTestFile(t, dir, "main.go", `x := os.Getenv("GO_KEY")`)

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	groups := result.GroupByLanguage()
	if len(groups) != 3 {
		t.Errorf("got %d languages, want 3: %v", len(groups), groups)
	}
	for lang, want := range map[string]string{"javascript": "JS_KEY", "python": "PY_KEY", "go": "GO_KEY"} {
		refs := groups[lang]
		if len(refs) != 1 || refs[0].Name != want {
			t.Errorf("groups[%q] = %+v, want one ref to %s", lang, refs, want)
		}
	}
}

// assertNames fails unless got contains exactly the wanted names, in order.
func assertNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			return
		}
	}
}

// writeTestFile is a test helper that writes content to a file named name
// inside dir. It marks the calling test as a helper for clean failure output.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
