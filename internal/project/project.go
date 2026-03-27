package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Project represents a registered project directory.
type Project struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	AddedAt    time.Time `json:"added_at"`
	LastPoured *time.Time `json:"last_poured,omitempty"`
}

// Registry manages the list of registered projects on disk.
type Registry struct {
	path     string // path to projects.json
	projects []Project
}

// NewRegistry creates a registry backed by the given JSON file.
func NewRegistry(path string) *Registry {
	return &Registry{path: path}
}

// Load reads the registry from disk.
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

// Save writes the registry to disk.
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

// Add registers a project directory. If it already exists, it's a no-op.
func (r *Registry) Add(dir string) (*Project, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Check if already registered
	for _, p := range r.projects {
		if p.Path == absPath {
			return &p, nil
		}
	}

	// Verify directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absPath)
	}

	name := filepath.Base(absPath)
	p := Project{
		Name:    name,
		Path:    absPath,
		AddedAt: time.Now().UTC(),
	}
	r.projects = append(r.projects, p)
	return &p, r.Save()
}

// Remove unregisters a project by path or name.
func (r *Registry) Remove(pathOrName string) error {
	absPath, _ := filepath.Abs(pathOrName)

	for i, p := range r.projects {
		if p.Path == absPath || p.Name == pathOrName {
			r.projects = append(r.projects[:i], r.projects[i+1:]...)
			return r.Save()
		}
	}
	return fmt.Errorf("project %q not found in registry", pathOrName)
}

// List returns all registered projects.
func (r *Registry) List() []Project {
	result := make([]Project, len(r.projects))
	copy(result, r.projects)
	return result
}

// Get returns a project by name or path.
func (r *Registry) Get(nameOrPath string) (*Project, bool) {
	absPath, _ := filepath.Abs(nameOrPath)
	for _, p := range r.projects {
		if p.Path == absPath || p.Name == nameOrPath {
			return &p, true
		}
	}
	return nil, false
}

// MarkPoured updates the last_poured timestamp for a project.
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

// FindEnvFiles scans a project directory for .env-related files.
func FindEnvFiles(dir string) []string {
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
