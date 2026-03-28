package sync

import (
	"errors"
	"time"
)

var (
	ErrNoRemoteConfigured = errors.New("no sync remote configured")
	ErrConflict           = errors.New("sync conflict detected")
	ErrRemoteNotFound     = errors.New("remote vault file not found")
)

// Backend is the interface for vault sync backends.
type Backend interface {
	// Push copies the local vault file to the remote location.
	Push(localPath string) error

	// Pull copies the remote vault file to the local location.
	Pull(localPath string) error

	// LastModified returns the remote file's last modification time.
	LastModified() (time.Time, error)

	// Name returns the backend identifier.
	Name() string
}

// Config holds sync configuration.
type Config struct {
	Backend    string `yaml:"backend"`     // "filesystem" or "git"
	RemotePath string `yaml:"remote_path"` // path for filesystem sync
	GitRemote  string `yaml:"git_remote"`  // git remote URL
	GitBranch  string `yaml:"git_branch"`  // git branch (default: "vial-vault")
	AutoSync   bool   `yaml:"auto_sync"`   // sync on every vault mutation
}

// Status represents the sync state between local and remote.
type Status struct {
	InSync        bool
	LocalModTime  time.Time
	RemoteModTime time.Time
	Backend       string
}
