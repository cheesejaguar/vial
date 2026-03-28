package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitBackend syncs the vault file via a dedicated git repository.
// The vault file is committed and pushed to a remote branch.
type GitBackend struct {
	repoDir  string // local git repo directory for sync
	remote   string // git remote URL
	branch   string // branch name (default: "vial-vault")
	fileName string // vault file name within the repo
}

// NewGitBackend creates a git sync backend.
func NewGitBackend(repoDir, remote, branch string) *GitBackend {
	if branch == "" {
		branch = "vial-vault"
	}
	return &GitBackend{
		repoDir:  repoDir,
		remote:   remote,
		branch:   branch,
		fileName: "vault.json",
	}
}

func (g *GitBackend) Name() string { return "git" }

// Push commits the local vault file and pushes to the remote.
func (g *GitBackend) Push(localPath string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	// Copy vault file into the repo
	repoFile := filepath.Join(g.repoDir, g.fileName)
	if err := copyFile(localPath, repoFile); err != nil {
		return fmt.Errorf("copying vault to repo: %w", err)
	}

	// Git add, commit, push
	if err := g.git("add", g.fileName); err != nil {
		return err
	}

	// Check if there are changes to commit
	output, err := g.gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "" {
		return nil // nothing to push
	}

	msg := fmt.Sprintf("vault sync %s", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if err := g.git("commit", "-m", msg); err != nil {
		return err
	}

	if g.remote == "" {
		return nil // local-only, no push
	}

	return g.git("push", "origin", g.branch)
}

// Pull fetches and checks out the remote vault file.
func (g *GitBackend) Pull(localPath string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	if err := g.git("fetch", "origin", g.branch); err != nil {
		return fmt.Errorf("fetching remote: %w", err)
	}

	if err := g.git("checkout", fmt.Sprintf("origin/%s", g.branch), "--", g.fileName); err != nil {
		return ErrRemoteNotFound
	}

	// Back up local, then copy from repo
	backupPath := localPath + ".bak"
	if _, err := os.Stat(localPath); err == nil {
		if err := copyFile(localPath, backupPath); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	repoFile := filepath.Join(g.repoDir, g.fileName)
	return copyFile(repoFile, localPath)
}

// LastModified returns the time of the last commit on the sync branch.
func (g *GitBackend) LastModified() (time.Time, error) {
	if err := g.ensureRepo(); err != nil {
		return time.Time{}, err
	}

	output, err := g.gitOutput("log", "-1", "--format=%aI", fmt.Sprintf("origin/%s", g.branch))
	if err != nil {
		return time.Time{}, ErrRemoteNotFound
	}

	t, err := time.Parse(time.RFC3339, strings.TrimSpace(output))
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing git timestamp: %w", err)
	}
	return t, nil
}

// ensureRepo initializes the git repo if it doesn't exist.
func (g *GitBackend) ensureRepo() error {
	if _, err := os.Stat(filepath.Join(g.repoDir, ".git")); err == nil {
		return nil // already initialized
	}

	if err := os.MkdirAll(g.repoDir, 0700); err != nil {
		return fmt.Errorf("creating repo directory: %w", err)
	}

	if err := g.git("init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Set a default git identity if none is configured (for CI/containers)
	g.git("config", "user.email", "vial@localhost")
	g.git("config", "user.name", "vial")

	if g.remote != "" {
		if err := g.git("remote", "add", "origin", g.remote); err != nil {
			// May already exist
			g.git("remote", "set-url", "origin", g.remote)
		}
	}

	// Create and checkout the sync branch
	g.git("checkout", "-b", g.branch)

	return nil
}

func (g *GitBackend) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitBackend) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoDir
	out, err := cmd.Output()
	return string(out), err
}
