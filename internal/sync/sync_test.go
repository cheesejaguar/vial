package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFilesystemPushPull exercises the full push → remote-modify → pull cycle
// for FilesystemBackend and verifies that a .bak backup is created before the
// local file is overwritten.
func TestFilesystemPushPull(t *testing.T) {
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	localPath := filepath.Join(localDir, "vault.json")
	remotePath := filepath.Join(remoteDir, "vault.json")

	content := []byte(`{"version": 1, "keys": {}}`)
	if err := os.WriteFile(localPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	backend := NewFilesystemBackend(remotePath)

	if backend.Name() != "filesystem" {
		t.Errorf("Name() = %q, want filesystem", backend.Name())
	}

	// Push
	if err := backend.Push(localPath); err != nil {
		t.Fatalf("Push: %v", err)
	}

	remoteData, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatalf("remote file not created: %v", err)
	}
	if string(remoteData) != string(content) {
		t.Error("remote content doesn't match")
	}

	// Modify remote
	modified := []byte(`{"version": 1, "keys": {"NEW_KEY": {}}}`)
	if err := os.WriteFile(remotePath, modified, 0600); err != nil {
		t.Fatal(err)
	}

	// Pull
	if err := backend.Pull(localPath); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	localData, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(localData) != string(modified) {
		t.Error("local content doesn't match after pull")
	}

	// Verify backup was created
	backupPath := localPath + ".bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(backupData) != string(content) {
		t.Error("backup content doesn't match original")
	}
}

// TestFilesystemPushCreatesRemoteDir verifies that Push creates any missing
// intermediate directories in the remote path.
func TestFilesystemPushCreatesRemoteDir(t *testing.T) {
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	localPath := filepath.Join(localDir, "vault.json")
	remotePath := filepath.Join(remoteDir, "sub", "dir", "vault.json")

	os.WriteFile(localPath, []byte("data"), 0600)

	backend := NewFilesystemBackend(remotePath)
	if err := backend.Push(localPath); err != nil {
		t.Fatalf("Push: %v", err)
	}

	data, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatalf("read remote: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("content = %q, want data", data)
	}
}

// TestFilesystemPullNoLocalBackup verifies that Pull does not create a .bak
// file when no local vault exists yet (initial bootstrap scenario).
func TestFilesystemPullNoLocalBackup(t *testing.T) {
	remoteDir := t.TempDir()
	localDir := t.TempDir()

	remotePath := filepath.Join(remoteDir, "vault.json")
	localPath := filepath.Join(localDir, "vault.json")

	// Remote exists, local does NOT — no backup should be created
	os.WriteFile(remotePath, []byte("remote-data"), 0600)

	backend := NewFilesystemBackend(remotePath)
	if err := backend.Pull(localPath); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	data, _ := os.ReadFile(localPath)
	if string(data) != "remote-data" {
		t.Errorf("content = %q", data)
	}

	// No backup should exist since there was no local file
	if _, err := os.Stat(localPath + ".bak"); !os.IsNotExist(err) {
		t.Error("backup should not exist when no local file existed")
	}
}

// TestFilesystemPullNotFound verifies that Pull returns ErrRemoteNotFound
// when the remote path does not exist.
func TestFilesystemPullNotFound(t *testing.T) {
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "vault.json")

	backend := NewFilesystemBackend("/nonexistent/vault.json")

	err := backend.Pull(localPath)
	if err != ErrRemoteNotFound {
		t.Errorf("expected ErrRemoteNotFound, got %v", err)
	}
}

// TestFilesystemLastModified verifies ErrRemoteNotFound before any push and a
// non-zero mod time after the remote file is created.
func TestFilesystemLastModified(t *testing.T) {
	dir := t.TempDir()
	remotePath := filepath.Join(dir, "vault.json")

	backend := NewFilesystemBackend(remotePath)

	_, err := backend.LastModified()
	if err != ErrRemoteNotFound {
		t.Errorf("expected ErrRemoteNotFound, got %v", err)
	}

	os.WriteFile(remotePath, []byte("test"), 0600)

	modTime, err := backend.LastModified()
	if err != nil {
		t.Fatalf("LastModified: %v", err)
	}
	if modTime.IsZero() {
		t.Error("modTime should not be zero")
	}
}

// TestCopyFileAtomic verifies that copyFile transfers all bytes correctly and
// leaves no .tmp file behind after a successful copy.
func TestCopyFileAtomic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	os.WriteFile(src, []byte("hello world"), 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "hello world" {
		t.Errorf("dst content = %q, want %q", data, "hello world")
	}

	// No temp file left behind
	tmpPath := dst + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up")
	}
}

// TestCopyFileSourceNotFound verifies that copyFile returns an error when the
// source path does not exist.
func TestCopyFileSourceNotFound(t *testing.T) {
	dir := t.TempDir()
	err := copyFile("/nonexistent/file", filepath.Join(dir, "dst"))
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

// TestCopyFilePreservesContent verifies that copyFile preserves every byte of
// binary data, including values across the full 0–255 range.
func TestCopyFilePreservesContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")

	// Write binary content
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(src, data, 0600)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if len(got) != len(data) {
		t.Fatalf("size = %d, want %d", len(got), len(data))
	}
	for i := range got {
		if got[i] != data[i] {
			t.Fatalf("byte %d: got %d, want %d", i, got[i], data[i])
		}
	}
}

// --- Git backend constructor tests ---

// TestNewGitBackendDefaults verifies that NewGitBackend fills in the default
// branch name ("vial-vault") and file name ("vault.json") when no branch is
// supplied.
func TestNewGitBackendDefaults(t *testing.T) {
	g := NewGitBackend("/tmp/repo", "git@github.com:user/vault.git", "")
	if g.Name() != "git" {
		t.Errorf("Name() = %q", g.Name())
	}
	if g.branch != "vial-vault" {
		t.Errorf("branch = %q, want vial-vault", g.branch)
	}
	if g.fileName != "vault.json" {
		t.Errorf("fileName = %q", g.fileName)
	}
}

// TestNewGitBackendCustomBranch verifies that an explicit branch name overrides
// the default.
func TestNewGitBackendCustomBranch(t *testing.T) {
	g := NewGitBackend("/tmp/repo", "", "custom-branch")
	if g.branch != "custom-branch" {
		t.Errorf("branch = %q, want custom-branch", g.branch)
	}
}

// TestGitBackendEnsureRepoCreatesDir verifies that ensureRepo initialises a
// git repository in a directory that does not exist yet, and that a second
// call on an already-initialised repo is a no-op.
func TestGitBackendEnsureRepoCreatesDir(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "new-repo")

	g := NewGitBackend(repoDir, "", "test-branch")
	err := g.ensureRepo()
	if err != nil {
		t.Fatalf("ensureRepo: %v", err)
	}

	// .git directory should exist
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Errorf(".git not created: %v", err)
	}

	// Calling again should be a no-op
	if err := g.ensureRepo(); err != nil {
		t.Fatalf("ensureRepo second call: %v", err)
	}
}

// TestGitBackendPushPullLocal exercises a full Push cycle using a local-only
// git backend (no remote configured). It verifies that the vault file is
// committed into the sync repo.
func TestGitBackendPushPullLocal(t *testing.T) {
	// Create a local vault file
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "vault.json")
	os.WriteFile(localPath, []byte(`{"version":1}`), 0600)

	// Create a git repo for sync
	repoDir := filepath.Join(t.TempDir(), "sync-repo")

	g := NewGitBackend(repoDir, "", "main")

	// Push should work (local repo, no remote push)
	if err := g.Push(localPath); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the file was committed
	repoFile := filepath.Join(repoDir, "vault.json")
	data, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("read repo file: %v", err)
	}
	if string(data) != `{"version":1}` {
		t.Errorf("repo file content = %q", data)
	}
}
