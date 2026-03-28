package cli

import (
	"fmt"

	"crypto/subtle"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/config"
	"github.com/cheesejaguar/vial/internal/vault"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new encrypted vault",
	Long:  "Create a new encrypted vault with a master password.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	fmt.Println(sectionHeader("🧪", "Initializing new vault"))
	fmt.Printf("  %s %s\n\n", arrowIcon(), mutedText(cfg.VaultPath))

	// Read password with confirmation
	pw1, err := readPassword("Enter master password (min 12 chars): ")
	if err != nil {
		return err
	}
	defer pw1.Destroy()

	if pw1.Size() < cfg.MinPasswordLen {
		return vault.ErrPasswordTooShort
	}

	pw2, err := readPassword("Confirm master password: ")
	if err != nil {
		return err
	}
	defer pw2.Destroy()

	if subtle.ConstantTimeCompare(pw1.Bytes(), pw2.Bytes()) != 1 {
		return fmt.Errorf("passwords do not match")
	}

	vm := vault.NewVaultManager(cfg.VaultPath)
	if err := vm.Init(pw1); err != nil {
		return err
	}
	defer vm.Lock()

	// Cache session
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

	// Create default config file if it doesn't exist
	configDir := config.DefaultConfigDir()
	createDefaultConfig(configDir)

	return nil
}

func createDefaultConfig(configDir string) {
	// Best-effort: create config dir and default config
	configPath := configDir + "/config.yaml"
	if _, err := readFileIfExists(configPath); err == nil {
		return // already exists
	}

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
