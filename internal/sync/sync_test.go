package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemPushPull(t *testing.T) {
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	localPath := filepath.Join(localDir, "vault.json")
	remotePath := filepath.Join(remoteDir, "vault.json")

	// Create a local vault file
	content := []byte(`{"version": 1, "keys": {}}`)
	if err := os.WriteFile(localPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	backend := NewFilesystemBackend(remotePath)

	// Push
	if err := backend.Push(localPath); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify remote exists
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

func TestFilesystemPullNotFound(t *testing.T) {
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "vault.json")

	backend := NewFilesystemBackend("/nonexistent/vault.json")

	err := backend.Pull(localPath)
	if err != ErrRemoteNotFound {
		t.Errorf("expected ErrRemoteNotFound, got %v", err)
	}
}

func TestFilesystemLastModified(t *testing.T) {
	dir := t.TempDir()
	remotePath := filepath.Join(dir, "vault.json")

	backend := NewFilesystemBackend(remotePath)

	// Not found
	_, err := backend.LastModified()
	if err != ErrRemoteNotFound {
		t.Errorf("expected ErrRemoteNotFound, got %v", err)
	}

	// Create file
	os.WriteFile(remotePath, []byte("test"), 0600)

	modTime, err := backend.LastModified()
	if err != nil {
		t.Fatalf("LastModified: %v", err)
	}
	if modTime.IsZero() {
		t.Error("modTime should not be zero")
	}
}

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
