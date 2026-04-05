// Package project manages the registry of project directories that a user has
// associated with their vault.
//
// A "project" in vial is simply a directory on disk that the user wants to
// pour secrets into.  The Registry persists this list as a JSON file
// (default: ~/.config/vial/projects.json) so that commands like "vial pour"
// can look up which .env file to populate without requiring the user to always
// specify the path explicitly.
//
// The registry records metadata alongside each path (display name, registration
// timestamp, last-poured timestamp) to power the dashboard project list and
// audit summaries.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Project represents a registered project directory and its associated metadata.
type Project struct {
	Name       string     `json:"name"`                  // display name derived from the directory basename
	Path       string     `json:"path"`                  // absolute path to the project root
	AddedAt    time.Time  `json:"added_at"`              // UTC time the project was registered
	LastPoured *time.Time `json:"last_poured,omitempty"` // UTC time secrets were last poured; nil if never
}

// Registry manages the list of registered projects, backed by a JSON file on disk.
// The zero value is not usable; create instances with NewRegistry.
type Registry struct {
	path     string    // absolute path to the projects.json file
	projects []Project // in-memory project list; populated by Load
}

// NewRegistry returns a Registry backed by the JSON file at path.
// The file is not read until Load is called.
func NewRegistry(path string) *Registry {
	return &Registry{path: path}
}

// Load reads the registry from disk into memory.
// If the file does not exist the registry is initialized as empty rather than
// returning an error, so callers do not need to special-case a brand-new
// installation.
func (r *Registry) Load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.projects = []Project{}
			return nil
		}
		return fmt.Errorf("reading project registry: %w", err)
	}

	if err := json.Unmarshal(data, &r.projects); err != nil {
		return fmt.Errorf("parsing project registry: %w", err)
	}
	return nil
}

// Save writes the current in-memory project list to disk as indented JSON.
// It creates the parent directory with restricted permissions (0700) if it
// does not already exist.
func (r *Registry) Save() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}

	data, err := json.MarshalIndent(r.projects, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling project registry: %w", err)
	}

	return os.WriteFile(r.path, data, 0644)
}

// Add registers the directory at dir, resolving it to an absolute path first.
// If the path is already in the registry the existing entry is returned unchanged
// (idempotent).  Returns an error if the path does not exist or is not a directory.
func (r *Registry) Add(dir string) (*Project, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Return early if this path is already tracked — duplicate registration is a
	// common user mistake and should not be an error.
	for _, p := range r.projects {
		if p.Path == absPath {
			return &p, nil
		}
	}

	// Verify the directory actually exists before persisting it.
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absPath)
	}

	// Use the directory's basename as the display name; users can rename via
	// the dashboard if the inferred name is not ideal.
	name := filepath.Base(absPath)
	p := Project{
		Name:    name,
		Path:    absPath,
		AddedAt: time.Now().UTC(),
	}
	r.projects = append(r.projects, p)
	return &p, r.Save()
}

// Remove unregisters a project identified by its absolute path or display name.
// Returns an error if no matching project is found.
func (r *Registry) Remove(pathOrName string) error {
	// Attempt an absolute-path match first; fall back to name comparison.
	absPath, _ := filepath.Abs(pathOrName)

	for i, p := range r.projects {
		if p.Path == absPath || p.Name == pathOrName {
			// Splice out the element while preserving order.
			r.projects = append(r.projects[:i], r.projects[i+1:]...)
			return r.Save()
		}
	}
	return fmt.Errorf("project %q not found in registry", pathOrName)
}

// List returns a snapshot copy of all registered projects.
// The returned slice is safe to mutate; changes do not affect the registry.
func (r *Registry) List() []Project {
	result := make([]Project, len(r.projects))
	copy(result, r.projects)
	return result
}

// Get returns a registered project by display name or absolute path.
// The second return value is false when no matching project is found.
func (r *Registry) Get(nameOrPath string) (*Project, bool) {
	absPath, _ := filepath.Abs(nameOrPath)
	for _, p := range r.projects {
		if p.Path == absPath || p.Name == nameOrPath {
			return &p, true
		}
	}
	return nil, false
}

// MarkPoured records that secrets were poured into path at the current time.
// This timestamp is displayed in the dashboard and in "vial shelf" output to
// help users see which projects have stale or missing secrets.
// If path is not registered the call is a no-op rather than an error, because
// "vial pour" can target unregistered directories.
func (r *Registry) MarkPoured(path string) error {
	absPath, _ := filepath.Abs(path)
	now := time.Now().UTC()
	for i, p := range r.projects {
		if p.Path == absPath {
			r.projects[i].LastPoured = &now
			return r.Save()
		}
	}
	return nil // silently ignore if not registered
}

// FindEnvFiles scans dir for the conventional .env file names used by popular
// frameworks and toolchains.  Only files that actually exist on disk are returned.
// The list is ordered by convention importance (plain .env first, then variants).
func FindEnvFiles(dir string) []string {
	// This set covers the naming conventions used by Node.js dotenv, Next.js,
	// Vite, Create React App, Docker Compose, and most CI systems.
	patterns := []string{
		".env",
		".env.example",
		".env.sample",
		".env.template",
		".env.local",
		".env.development",
		".env.production",
		".env.test",
	}

	var found []string
	for _, p := range patterns {
		path := filepath.Join(dir, p)
		if _, err := os.Stat(path); err == nil {
			found = append(found, p)
		}
	}
	return found
}
