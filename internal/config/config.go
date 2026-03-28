package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	VaultPath      string        `mapstructure:"vault_path"`
	SessionTimeout time.Duration `mapstructure:"session_timeout"`
	EnvExample     string        `mapstructure:"env_example"`
	LogLevel       string        `mapstructure:"log_level"`
	MinPasswordLen int           `mapstructure:"min_password_length"`
}

// DefaultConfig returns the configuration with all defaults applied.
func DefaultConfig() Config {
	return Config{
		VaultPath:      defaultVaultPath(),
		SessionTimeout: 4 * time.Hour,
		EnvExample:     ".env.example",
		LogLevel:       "warn",
		MinPasswordLen: 12,
	}
}

func defaultVaultPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "vial", "vault.json")
}

// DefaultConfigDir returns the default config directory path.
func DefaultConfigDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "vial")
}

// Load reads config from the YAML file, env vars, and flags.
// Precedence: flag > env var > config file > default.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	defaults := DefaultConfig()
	v.SetDefault("vault_path", defaults.VaultPath)
	v.SetDefault("session_timeout", defaults.SessionTimeout)
	v.SetDefault("env_example", defaults.EnvExample)
	v.SetDefault("log_level", defaults.LogLevel)
	v.SetDefault("min_password_length", defaults.MinPasswordLen)

	v.SetEnvPrefix("VIAL")
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(DefaultConfigDir())
	}

	if err := v.ReadInConfig(); err != nil {
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
