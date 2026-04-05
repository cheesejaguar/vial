package project

import (
	"os"
	"path/filepath"
	"testing"
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
	os.Mkdir(proj1, 0755)
	os.Mkdir(proj2, 0755)

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
}

// TestRegistryAddDuplicate confirms that registering the same directory twice
// is a no-op and does not create a second entry.
func TestRegistryAddDuplicate(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	r := NewRegistry(regPath)
	r.Load()

	projDir := filepath.Join(dir, "myproject")
	os.Mkdir(projDir, 0755)

	r.Add(projDir)
	r.Add(projDir) // duplicate

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
	os.Mkdir(projDir, 0755)

	// Write
	r1 := NewRegistry(regPath)
	r1.Load()
	r1.Add(projDir)

	// Read fresh
	r2 := NewRegistry(regPath)
	if err := r2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	list := r2.List()
	if len(list) != 1 {
		t.Fatalf("persisted list len = %d, want 1", len(list))
	}
	if list[0].Name != "myproject" {
		t.Errorf("Name = %q, want myproject", list[0].Name)
	}
}

// TestFindEnvFiles verifies that only files actually present on disk are returned.
func TestFindEnvFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some env files
	for _, name := range []string{".env", ".env.example", ".env.local"} {
		os.WriteFile(filepath.Join(dir, name), []byte("KEY=val"), 0644)
	}

	found := FindEnvFiles(dir)
	if len(found) != 3 {
		t.Fatalf("found %d files, want 3: %v", len(found), found)
	}
}

// TestRegistryMarkPoured verifies that the LastPoured timestamp is set on
// a registered project after a pour operation.
func TestRegistryMarkPoured(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "projects.json")
	projDir := filepath.Join(dir, "myproject")
	os.Mkdir(projDir, 0755)

	r := NewRegistry(regPath)
	r.Load()
	r.Add(projDir)

	if err := r.MarkPoured(projDir); err != nil {
		t.Fatalf("MarkPoured: %v", err)
	}

	p, _ := r.Get("myproject")
	if p.LastPoured == nil {
		t.Error("LastPoured should be set")
	}
}
