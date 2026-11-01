package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRegistryAddListRemove exercises the core CRUD operations: adding two
// projects, listing them, retrieving one by name, and removing one.
func TestRegistryAddListRemove(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	r := NewRegistry(regPath)

	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Create project directories
	proj1 := filepath.Join(dir, "project1")
	proj2 := filepath.Join(dir, "project2")
	mustMkdir(t, proj1)
	mustMkdir(t, proj2)

	// Add
	p1, err := r.Add(proj1)
	if err != nil {
		t.Fatalf("Add proj1: %v", err)
	}
	if p1.Name != "project1" {
		t.Errorf("Name = %q, want project1", p1.Name)
	}

	p2, err := r.Add(proj2)
	if err != nil {
		t.Fatalf("Add proj2: %v", err)
	}
	if p2.Name != "project2" {
		t.Errorf("Name = %q, want project2", p2.Name)
	}

	// List
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}

	// Get
	p, ok := r.Get("project1")
	if !ok || p.Path != proj1 {
		t.Errorf("Get(project1) = %v, %v", p, ok)
	}

	// Remove
	if err := r.Remove("project1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	list = r.List()
	if len(list) != 1 {
		t.Fatalf("List after remove = %d, want 1", len(list))
	}
	if list[0].Name != "project2" {
		t.Errorf("remaining project = %q, want project2", list[0].Name)
	}
	if _, ok := r.Get("project1"); ok {
		t.Error("Get(project1) should fail after Remove")
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
}

// TestRegistryAddDuplicate confirms that registering the same directory twice
// is a no-op and does not create a second entry.
func TestRegistryAddDuplicate(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	r := NewRegistry(regPath)
	if err := r.Load(); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(dir, "myproject")
	mustMkdir(t, projDir)

	if _, err := r.Add(projDir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := r.Add(projDir); err != nil { // duplicate
		t.Fatalf("duplicate Add: %v", err)
	}

	if len(r.List()) != 1 {
		t.Error("duplicate add should be a no-op")
	}
}

// TestRegistryAddNonexistentDir verifies that Add rejects a path that does not
// exist on the filesystem.
func TestRegistryAddNonexistentDir(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(filepath.Join(dir, "projects.json"))
	r.Load()

	_, err := r.Add("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// TestRegistryPersistence ensures that a project added in one Registry instance
// is visible after constructing a second instance that loads from the same file.
func TestRegistryPersistence(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	projDir := filepath.Join(dir, "myproject")
	mustMkdir(t, projDir)

	// Write
	r1 := NewRegistry(regPath)
	if err := r1.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := r1.Add(projDir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Read fresh
	r2 := NewRegistry(regPath)
	if err := r2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	list := r2.List()
	if len(list) != 1 {
		t.Fatalf("persisted list len = %d, want 1", len(list))
	}
	if list[0].Name != "myproject" || list[0].Path != projDir {
		t.Errorf("persisted project = %+v, want myproject at %s", list[0], projDir)
	}
}

// TestFindEnvFiles verifies that only conventional env files actually present
// on disk are returned, in convention order, and unrelated files are ignored.
func TestFindEnvFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some env files (out of order) plus a non-conventional name
	for _, name := range []string{".env.local", ".env", ".env.example", ".env.backup"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("KEY=val"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	found := FindEnvFiles(dir)
	want := []string{".env", ".env.example", ".env.local"}
	if len(found) != len(want) {
		t.Fatalf("found %v, want %v", found, want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Errorf("found %v, want %v", found, want)
			break
		}
	}
}

// TestRegistryMarkPoured verifies that the LastPoured timestamp is set on
// a registered project after a pour operation.
func TestRegistryMarkPoured(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	projDir := filepath.Join(dir, "myproject")
	mustMkdir(t, projDir)

	r := NewRegistry(regPath)
	if err := r.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(projDir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	before := time.Now()
	if err := r.MarkPoured(projDir); err != nil {
		t.Fatalf("MarkPoured: %v", err)
	}

	p, ok := r.Get("myproject")
	if !ok {
		t.Fatal("project not found")
	}
	if p.LastPoured == nil {
		t.Fatal("LastPoured should be set")
	}
	if p.LastPoured.Before(before.Add(-time.Second)) {
		t.Errorf("LastPoured = %v, want approximately now", p.LastPoured)
	}
}
