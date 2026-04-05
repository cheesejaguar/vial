// Package config provides Vial's application-level configuration backed by
// Viper. Settings are resolved from up to four sources in descending priority:
//
//  1. CLI flags  — passed via cobra's PersistentFlags, bound externally.
//  2. Environment variables — prefixed with "VIAL_" (e.g. VIAL_VAULT_PATH).
//  3. YAML config file — read from ~/.config/vial/config.yaml by default, or
//     from the path supplied via the --config flag.
//  4. Built-in defaults — returned by DefaultConfig() and registered with
//     Viper so they apply when a key is absent from all other sources.
//
// File locations follow the XDG Base Directory Specification:
//   - Config: $XDG_CONFIG_HOME/vial/config.yaml  (default ~/.config/vial/)
//   - Vault:  $XDG_DATA_HOME/vial/vault.json     (default ~/.local/share/vial/)
//
// Both XDG variables fall back to their conventional defaults (~/.config and
// ~/.local/share) when unset, which matches the expected behaviour on Linux,
// macOS, and WSL environments.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration values resolved from the YAML
// file, environment variables, and built-in defaults. The mapstructure tags
// match the YAML key names so that Viper.Unmarshal populates the struct
// correctly regardless of whether the value came from a file or env var.
type Config struct {
	// VaultPath is the absolute path to the AES-256-GCM encrypted vault file.
	// Defaults to $XDG_DATA_HOME/vial/vault.json.
	VaultPath string `mapstructure:"vault_path"`

	// SessionTimeout controls how long the DEK is cached in the OS keyring
	// after a successful password unlock. Once expired the user must re-enter
	// the master password. Default: 4h.
	SessionTimeout time.Duration `mapstructure:"session_timeout"`

	// EnvExample is the filename of the template file that lists the required
	// env vars for a project. Used by "vial pour" as the source of keys to
	// populate and by "vial scaffold" as the output filename. Default: .env.example.
	EnvExample string `mapstructure:"env_example"`

	// LogLevel controls the minimum log level written to stderr. Accepted
	// values mirror the charmbracelet/log levels: debug, info, warn, error.
	// Default: warn.
	LogLevel string `mapstructure:"log_level"`

	// MinPasswordLen is the minimum number of bytes required for a new master
	// password when running "vial init". Default: 12.
	MinPasswordLen int `mapstructure:"min_password_length"`
}

// DefaultConfig returns a Config populated with all built-in defaults. It is
// used both to register Viper defaults and as a convenient source of truth
// when documenting expected values.
func DefaultConfig() Config {
	return Config{
		VaultPath:      defaultVaultPath(),
		SessionTimeout: 4 * time.Hour,
		EnvExample:     ".env.example",
		LogLevel:       "warn",
		MinPasswordLen: 12,
	}
}

// defaultVaultPath resolves the XDG data home directory and appends the
// vial-specific subdirectory and filename. It is intentionally unexported
// because external callers should use DefaultConfig().VaultPath.
func defaultVaultPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		// XDG spec: fall back to $HOME/.local/share when XDG_DATA_HOME is unset.
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "vial", "vault.json")
}

// DefaultConfigDir returns the directory where Vial stores its config.yaml.
// It follows the XDG Base Directory Specification, falling back to
// ~/.config/vial when XDG_CONFIG_HOME is not set. The function is exported so
// that "vial init" can create the directory and write the default config file.
func DefaultConfigDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		// XDG spec: fall back to $HOME/.config when XDG_CONFIG_HOME is unset.
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "vial")
}

// Load reads the application configuration and returns a fully resolved Config.
//
// Resolution order (highest to lowest precedence):
//  1. VIAL_* environment variables (e.g. VIAL_VAULT_PATH, VIAL_SESSION_TIMEOUT).
//  2. The YAML file at configPath when non-empty, or config.yaml inside
//     DefaultConfigDir() when configPath is empty.
//  3. Built-in defaults from DefaultConfig().
//
// A missing config file is not an error — Vial operates entirely on defaults
// when the file does not exist, which is the expected state after a fresh
// install. Only a malformed (unparseable) config file returns an error.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Register all defaults up front so Viper returns them for any key not
	// overridden by a file or environment variable.
	defaults := DefaultConfig()
	v.SetDefault("vault_path", defaults.VaultPath)
	v.SetDefault("session_timeout", defaults.SessionTimeout)
	v.SetDefault("env_example", defaults.EnvExample)
	v.SetDefault("log_level", defaults.LogLevel)
	v.SetDefault("min_password_length", defaults.MinPasswordLen)

	// Environment variable bindings. AutomaticEnv maps every config key to a
	// corresponding VIAL_<UPPER_KEY> variable, so VIAL_VAULT_PATH overrides
	// vault_path without requiring explicit BindEnv calls per key.
	v.SetEnvPrefix("VIAL")
	v.AutomaticEnv()

	if configPath != "" {
		// Explicit path provided (e.g. via --config flag): use it directly so
		// the user can maintain multiple config profiles.
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(DefaultConfigDir())
	}

	if err := v.ReadInConfig(); err != nil {
		// Distinguish between "file not found" (acceptable) and "file exists
		// but is malformed" (hard error). Both viper.ConfigFileNotFoundError
		// and os.IsNotExist cover the two ways a missing file is reported
		// depending on whether Viper found the path before trying to open it.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config: %w", err)
			}
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}
