package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirJavaScript(t *testing.T) {
	dir := t.TempDir()

	// Create a JS file with env var references
	js := `
const apiKey = process.env.OPENAI_API_KEY;
const dbUrl = process.env["DATABASE_URL"];
const port = process.env.PORT;
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

	names := result.UniqueVarNames()
	if len(names) != 3 {
		t.Errorf("got %d vars, want 3: %v", len(names), names)
	}
}

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

	names := result.UniqueVarNames()
	if len(names) != 2 {
		t.Errorf("got %d vars, want 2: %v", len(names), names)
	}
}

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

	if len(result.UniqueVarNames()) != 2 {
		t.Errorf("got %d vars, want 2", len(result.UniqueVarNames()))
	}
}

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

	missing := result.FilterMissing(have)
	if len(missing) != 2 {
		t.Errorf("got %d missing, want 2: %v", len(missing), missing)
	}
}

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
		t.Errorf("got %d languages, want 3", len(groups))
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
