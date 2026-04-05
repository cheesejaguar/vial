package cli

import (
	"fmt"

	"crypto/subtle"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/config"
	"github.com/cheesejaguar/vial/internal/vault"
)

// initCmd creates a brand-new encrypted vault at the configured vault_path.
// It is the entry point for first-time users and must be run before any other
// command that requires an unlocked vault.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new encrypted vault",
	Long:  "Create a new encrypted vault with a master password.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

// runInit implements the "vial init" command. It:
//  1. Prompts for a master password twice and compares them with
//     crypto/subtle.ConstantTimeCompare to prevent timing-based oracle attacks.
//  2. Delegates vault creation (key derivation + DEK generation + file write)
//     to vault.VaultManager.Init.
//  3. Seeds the OS keyring session cache immediately after creation so the user
//     does not have to re-enter the password for the first batch of commands.
//  4. Writes a commented-out default config file if one does not already exist,
//     giving new users a starting point for customisation.
func runInit(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	fmt.Println(sectionHeader("🧪", "Initializing new vault"))
	fmt.Printf("  %s %s\n\n", arrowIcon(), mutedText(cfg.VaultPath))

	// Read the master password twice. echo is disabled for both reads so the
	// password never appears in the terminal scrollback.
	pw1, err := readPassword("Enter master password (min 12 chars): ")
	if err != nil {
		return err
	}
	defer pw1.Destroy()

	// Enforce the minimum length before prompting for confirmation to give
	// immediate feedback rather than making the user type a second time.
	if pw1.Size() < cfg.MinPasswordLen {
		return vault.ErrPasswordTooShort
	}

	pw2, err := readPassword("Confirm master password: ")
	if err != nil {
		return err
	}
	defer pw2.Destroy()

	// Constant-time comparison prevents a timing side-channel that could leak
	// partial information about pw1 if a fast-path exit existed on mismatch.
	if subtle.ConstantTimeCompare(pw1.Bytes(), pw2.Bytes()) != 1 {
		return fmt.Errorf("passwords do not match")
	}

	vm := vault.NewVaultManager(cfg.VaultPath)
	if err := vm.Init(pw1); err != nil {
		return err
	}
	defer vm.Lock()

	// Cache the DEK in the keyring immediately so commands run right after
	// "vial init" (e.g. in a setup script) do not prompt for the password.
	if dekBytes := vm.DEKBytes(); dekBytes != nil {
		if err := session.Store(cfg.VaultPath, dekBytes, cfg.SessionTimeout); err != nil {
			logger.Warn("Could not cache session in keyring", "err", err)
		}
	}

	fmt.Println()
	fmt.Println(successMsg("Vault created successfully! ✨"))
	fmt.Println()
	fmt.Println(headerText("Get started:"))
	fmt.Printf("  %s  %s\n", keyName("vial key set OPENAI_API_KEY"), mutedText("Add a secret"))
	fmt.Printf("  %s  %s\n", keyName("vial key list"), mutedText("             List stored keys"))
	fmt.Printf("  %s  %s\n", keyName("vial pour"), mutedText("                  Populate .env from vault"))

	// Write a default config file alongside the vault. Errors are non-fatal
	// because the default values baked into DefaultConfig() are sufficient to
	// run all commands without a file on disk.
	configDir := config.DefaultConfigDir()
	createDefaultConfig(configDir)

	return nil
}

// createDefaultConfig writes a minimal commented-out config.yaml to configDir
// if the file does not already exist. The file is intentionally all-comments so
// it serves as self-documenting documentation of available options without
// overriding any defaults. Errors are logged at debug level and not propagated
// because a missing config file is not fatal.
func createDefaultConfig(configDir string) {
	configPath := configDir + "/config.yaml"
	if _, err := readFileIfExists(configPath); err == nil {
		return // file already exists; do not overwrite user customisations
	}

	// All keys are commented out so the file documents the available settings
	// without changing any runtime behaviour (defaults remain in effect).
	content := fmt.Sprintf(`# Vial configuration
# vault_path: %s
# session_timeout: 4h
# env_example: .env.example
# log_level: warn
`, cfg.VaultPath)

	if err := writeFileWithDirs(configPath, []byte(content), 0644); err != nil {
		logger.Debug("Could not create default config", "err", err)
	}
}
