package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	vialsync "github.com/cheesejaguar/vial/internal/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the vault to/from a remote location",
}

var syncPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push the local vault to the remote",
	RunE:  runSyncPush,
}

var syncPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull the remote vault to local",
	RunE:  runSyncPull,
}

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE:  runSyncStatus,
}

var (
	syncBackend string
	syncRemote  string
)

func init() {
	syncCmd.PersistentFlags().StringVar(&syncBackend, "backend", "filesystem", "Sync backend (filesystem, git)")
	syncCmd.PersistentFlags().StringVar(&syncRemote, "remote", "", "Remote path or URL")
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncPullCmd)
	syncCmd.AddCommand(syncStatusCmd)
	rootCmd.AddCommand(syncCmd)
}

func getSyncBackend() (vialsync.Backend, error) {
	if err := loadConfig(); err != nil {
		return nil, err
	}

	if syncRemote == "" {
		return nil, fmt.Errorf("--remote is required (path for filesystem, URL for git)")
	}

	switch syncBackend {
	case "filesystem":
		return vialsync.NewFilesystemBackend(syncRemote), nil
	case "git":
		repoDir := filepath.Join(filepath.Dir(cfg.VaultPath), ".vial-sync")
		return vialsync.NewGitBackend(repoDir, syncRemote, "vial-vault"), nil
	default:
		return nil, fmt.Errorf("unknown sync backend: %s", syncBackend)
	}
}

func runSyncPush(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	backend, err := getSyncBackend()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cfg.VaultPath); os.IsNotExist(err) {
		return fmt.Errorf("no local vault found at %s", cfg.VaultPath)
	}

	fmt.Printf("Pushing vault via %s...\n", backend.Name())
	if err := backend.Push(cfg.VaultPath); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	fmt.Println("✓ Vault pushed successfully")
	return nil
}

func runSyncPull(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	backend, err := getSyncBackend()
	if err != nil {
		return err
	}

	fmt.Printf("Pulling vault via %s...\n", backend.Name())
	if err := backend.Pull(cfg.VaultPath); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	fmt.Println("✓ Vault pulled successfully")
	if _, err := os.Stat(cfg.VaultPath + ".bak"); err == nil {
		fmt.Println("  Previous vault backed up to vault.json.bak")
	}
	return nil
}

func runSyncStatus(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	backend, err := getSyncBackend()
	if err != nil {
		return err
	}

	// Local info
	localInfo, err := os.Stat(cfg.VaultPath)
	if err != nil {
		fmt.Println("Local vault: not found")
	} else {
		fmt.Printf("Local vault: %s (modified %s)\n", cfg.VaultPath, localInfo.ModTime().Format(time.RFC3339))
	}

	// Remote info
	remoteTime, err := backend.LastModified()
	if err != nil {
		fmt.Printf("Remote (%s): not found or unreachable\n", backend.Name())
	} else {
		fmt.Printf("Remote (%s): modified %s\n", backend.Name(), remoteTime.Format(time.RFC3339))

		if localInfo != nil {
			if localInfo.ModTime().After(remoteTime) {
				fmt.Println("Status: local is newer → run 'vial sync push'")
			} else if remoteTime.After(localInfo.ModTime()) {
				fmt.Println("Status: remote is newer → run 'vial sync pull'")
			} else {
				fmt.Println("Status: in sync")
			}
		}
	}

	return nil
}
