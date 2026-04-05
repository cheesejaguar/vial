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
//
// The backend maintains a local git repository at repoDir that contains a
// single file — vault.json. On Push, the vault file is copied into the repo,
// committed with a timestamped message, and pushed to the configured remote
// branch. On Pull, the remote branch is fetched and the vault file is checked
// out into the local working copy before being copied back to the caller's
// localPath.
//
// This backend provides a full history of vault changes at the cost of
// requiring git to be installed and a reachable remote. If no remote is
// configured, commits are made locally only (useful for testing or offline
// workflows).
//
// The dedicated repo approach keeps the vault history completely isolated from
// any application code repository, avoiding accidental secret exposure through
// git log or diff in a shared repo.
type GitBackend struct {
	repoDir  string // absolute path to the local sync git repository
	remote   string // git remote URL; empty string means local-only (no push/pull)
	branch   string // branch name used for the vault file (default: "vial-vault")
	fileName string // vault file name within the repo (always "vault.json")
}

// NewGitBackend creates a git sync backend.
//
// repoDir is the local directory used as the git repository. It need not exist
// yet; ensureRepo will create and initialise it on the first Push or Pull.
// remote is the git remote URL (may be empty for local-only operation).
// branch is the branch name; if empty, it defaults to "vial-vault".
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

// Name returns the backend identifier "git".
func (g *GitBackend) Name() string { return "git" }

// Push commits the local vault file to the sync repository and pushes it to
// the configured remote.
//
// If the vault file has not changed since the last commit (git status reports
// a clean working tree after staging), Push is a no-op. If no remote is
// configured, the commit is created locally but no push is attempted.
func (g *GitBackend) Push(localPath string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	// Copy the caller's vault file into the managed repo directory.
	repoFile := filepath.Join(g.repoDir, g.fileName)
	if err := copyFile(localPath, repoFile); err != nil {
		return fmt.Errorf("copying vault to repo: %w", err)
	}

	// Stage the vault file.
	if err := g.git("add", g.fileName); err != nil {
		return err
	}

	// Check whether staging produced any changes to commit. git status
	// --porcelain outputs nothing when the working tree is clean.
	output, err := g.gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "" {
		// Vault content is identical to the last committed version; skip commit.
		return nil
	}

	// Use an ISO-8601 UTC timestamp in the commit message so the history is
	// human-readable without additional tooling.
	msg := fmt.Sprintf("vault sync %s", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if err := g.git("commit", "-m", msg); err != nil {
		return err
	}

	if g.remote == "" {
		// No remote configured; local commit is sufficient.
		return nil
	}

	return g.git("push", "origin", g.branch)
}

// Pull fetches the remote branch and copies the vault file to localPath.
//
// A .bak backup of the existing local vault is created before overwriting it.
// If the remote branch or vault file does not exist, ErrRemoteNotFound is
// returned so the caller knows a push is needed rather than a pull.
func (g *GitBackend) Pull(localPath string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	// Fetch only the configured branch to minimise network traffic.
	if err := g.git("fetch", "origin", g.branch); err != nil {
		return fmt.Errorf("fetching remote: %w", err)
	}

	// Check out the vault file from the remote branch into the repo working
	// directory without advancing HEAD, so the local branch history stays clean.
	if err := g.git("checkout", fmt.Sprintf("origin/%s", g.branch), "--", g.fileName); err != nil {
		return ErrRemoteNotFound
	}

	// Back up the existing local vault before overwriting it.
	backupPath := localPath + ".bak"
	if _, err := os.Stat(localPath); err == nil {
		if err := copyFile(localPath, backupPath); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	repoFile := filepath.Join(g.repoDir, g.fileName)
	return copyFile(repoFile, localPath)
}

// LastModified returns the author timestamp of the most recent commit on the
// remote sync branch. This is used by the CLI to determine whether a push or
// pull is needed.
//
// Returns ErrRemoteNotFound if the remote branch has no commits yet.
func (g *GitBackend) LastModified() (time.Time, error) {
	if err := g.ensureRepo(); err != nil {
		return time.Time{}, err
	}

	// --format=%aI uses ISO 8601 strict format with timezone offset, which
	// Go's time.RFC3339 parser handles correctly.
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

// ensureRepo initialises the git repository at repoDir if it does not already
// exist.
//
// On first call it runs "git init", configures a fallback identity (needed in
// CI environments where no global git config is set), adds the remote, and
// creates the sync branch. Subsequent calls are no-ops detected by the presence
// of the .git directory. The directory is created with 0700 permissions to
// prevent other users from reading the vault history.
func (g *GitBackend) ensureRepo() error {
	if _, err := os.Stat(filepath.Join(g.repoDir, ".git")); err == nil {
		// Repository already initialised; nothing to do.
		return nil
	}

	if err := os.MkdirAll(g.repoDir, 0700); err != nil {
		return fmt.Errorf("creating repo directory: %w", err)
	}

	if err := g.git("init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Set a default git identity so commits succeed in CI containers and
	// environments where no global user.name / user.email is configured.
	// These values are intentionally generic; they appear only in the private
	// sync repo history and have no security significance.
	g.git("config", "user.email", "vial@localhost")
	g.git("config", "user.name", "vial")

	if g.remote != "" {
		if err := g.git("remote", "add", "origin", g.remote); err != nil {
			// The remote may already exist if ensureRepo is called more than
			// once with a partially initialised repo; update the URL instead.
			g.git("remote", "set-url", "origin", g.remote)
		}
	}

	// Create and switch to the sync branch. This branch is dedicated entirely
	// to vault storage; it should never be merged into an application repo.
	g.git("checkout", "-b", g.branch)

	return nil
}

// git runs a git command inside g.repoDir, directing both stdout and stderr to
// os.Stderr so that progress messages and errors are visible to the user. The
// return value is the exit error from the git subprocess.
func (g *GitBackend) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoDir
	// Route git's own output to our stderr rather than swallowing it; this
	// ensures that authentication prompts and push progress are visible.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitOutput runs a git command inside g.repoDir and returns its combined
// stdout as a string. Stderr is discarded so that the caller receives only the
// machine-readable output (e.g. a single-line timestamp or status code).
func (g *GitBackend) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoDir
	out, err := cmd.Output()
	return string(out), err
}
