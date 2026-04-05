package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FilesystemBackend syncs the vault file via a plain filesystem copy.
//
// This backend is suitable for any directory that is kept in sync by an
// external agent such as iCloud Drive, Dropbox, Google Drive, or a mounted
// network share. It does not manage any transport itself — it simply copies
// the vault file to a configured path and trusts the underlying storage layer
// to propagate the change to other devices.
//
// All writes go through copyFile, which uses a temp-file rename so that the
// destination is never left in a partially written state.
type FilesystemBackend struct {
	remotePath string // full path to the remote vault file, set at construction time
}

// NewFilesystemBackend creates a filesystem sync backend that reads and writes
// the vault file at remotePath.
func NewFilesystemBackend(remotePath string) *FilesystemBackend {
	return &FilesystemBackend{remotePath: remotePath}
}

// Name returns the backend identifier "filesystem".
func (f *FilesystemBackend) Name() string { return "filesystem" }

// Push copies the local vault file to the configured remote path.
//
// If the remote directory does not exist it is created with 0700 permissions
// (owner-only, matching the vault file itself). The copy is atomic: the data
// is written to a temporary file first and then renamed into place.
func (f *FilesystemBackend) Push(localPath string) error {
	if err := os.MkdirAll(filepath.Dir(f.remotePath), 0700); err != nil {
		return fmt.Errorf("creating remote directory: %w", err)
	}

	return copyFile(localPath, f.remotePath)
}

// Pull copies the remote vault file to localPath.
//
// If a local vault already exists, it is backed up to localPath+".bak" before
// being overwritten. This gives the user a recovery option if the remote
// contains unexpected changes. Returns ErrRemoteNotFound when the remote file
// does not exist yet.
func (f *FilesystemBackend) Pull(localPath string) error {
	if _, err := os.Stat(f.remotePath); os.IsNotExist(err) {
		return ErrRemoteNotFound
	}

	// Back up the current local vault before overwriting it.
	backupPath := localPath + ".bak"
	if _, err := os.Stat(localPath); err == nil {
		if err := copyFile(localPath, backupPath); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	return copyFile(f.remotePath, localPath)
}

// LastModified returns the modification time of the remote vault file.
// Returns ErrRemoteNotFound when the remote file does not exist.
func (f *FilesystemBackend) LastModified() (time.Time, error) {
	info, err := os.Stat(f.remotePath)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, ErrRemoteNotFound
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// copyFile copies src to dst atomically using a temp-file rename.
//
// The destination is written to dst+".tmp" first; only on a successful close
// is the temp file renamed to dst. This ensures dst is never observable in a
// partially written state by concurrent readers. The temp file is removed on
// any error so no stale files are left behind. The destination inherits the
// file mode of the source.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Write to a sibling temp file so the rename is guaranteed to be atomic
	// on POSIX systems (same filesystem, single directory entry swap).
	tmp := dst + ".tmp"
	dstFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(tmp) // clean up partial write
		return fmt.Errorf("copying data: %w", err)
	}

	// Close before rename so all buffered data is flushed to disk.
	if err := dstFile.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	// Atomic rename: readers will see either the old file or the new one,
	// never a partial state.
	return os.Rename(tmp, dst)
}
