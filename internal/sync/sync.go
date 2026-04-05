// Package sync provides backend implementations for synchronising the vial
// vault file to a remote location.
//
// Two backends are currently supported:
//
//   - FilesystemBackend: copies the vault file to any path accessible from the
//     local filesystem (iCloud Drive, Dropbox, network share, etc.).
//   - GitBackend: commits the vault file into a dedicated git repository and
//     pushes it to a configured remote.
//
// Both backends follow a push/pull model. The caller is responsible for
// deciding when to sync; AutoSync in Config is a hint for the CLI layer.
// All file copies are performed atomically via a temp-file rename to
// avoid leaving a partially written vault file in the remote location.
package sync

import (
	"errors"
	"time"
)

// ErrNoRemoteConfigured is returned when a sync operation is attempted but no
// remote location has been configured.
var ErrNoRemoteConfigured = errors.New("no sync remote configured")

// ErrConflict is returned when both the local and remote vault files have been
// modified since the last sync and an automatic merge is not possible.
var ErrConflict = errors.New("sync conflict detected")

// ErrRemoteNotFound is returned when the remote vault file does not exist,
// typically on the first pull before any push has occurred.
var ErrRemoteNotFound = errors.New("remote vault file not found")

// Backend is the interface implemented by every sync backend.
// All methods operate on the vault file identified by localPath.
type Backend interface {
	// Push copies the local vault file to the remote location.
	// If the remote directory does not exist, Push must create it.
	Push(localPath string) error

	// Pull copies the remote vault file to the local location.
	// If a local file already exists, the backend must create a .bak backup
	// before overwriting. Returns ErrRemoteNotFound when the remote has no
	// vault file yet.
	Pull(localPath string) error

	// LastModified returns the modification time of the remote vault file.
	// Returns ErrRemoteNotFound when the remote has no vault file.
	LastModified() (time.Time, error)

	// Name returns a short human-readable identifier for the backend
	// (e.g. "filesystem" or "git"), used in log and status messages.
	Name() string
}

// Config holds the sync configuration loaded from the vial YAML config file.
// It is intentionally kept flat so it maps cleanly to a single YAML block.
type Config struct {
	Backend    string `yaml:"backend"`     // "filesystem" or "git"
	RemotePath string `yaml:"remote_path"` // absolute or home-relative path for filesystem sync
	GitRemote  string `yaml:"git_remote"`  // git remote URL (e.g. "git@github.com:user/vault.git")
	GitBranch  string `yaml:"git_branch"`  // branch name; defaults to "vial-vault" when empty
	AutoSync   bool   `yaml:"auto_sync"`   // when true, the CLI syncs on every vault mutation
}

// Status represents the sync state between the local vault and the remote.
// It is returned by higher-level sync helpers and surfaced in the CLI's
// status output.
type Status struct {
	InSync        bool      // true when local and remote modification times are equal
	LocalModTime  time.Time // modification time of the local vault file
	RemoteModTime time.Time // modification time of the remote vault file
	Backend       string    // name of the active backend (from Backend.Name)
}
