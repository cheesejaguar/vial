package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SessionTimeout != 4*time.Hour {
		t.Errorf("SessionTimeout = %v, want 4h", cfg.SessionTimeout)
	}
	if cfg.EnvExample != ".env.example" {
		t.Errorf("EnvExample = %q, want .env.example", cfg.EnvExample)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn", cfg.LogLevel)
	}
	if cfg.MinPasswordLen != 12 {
		t.Errorf("MinPasswordLen = %d, want 12", cfg.MinPasswordLen)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.SessionTimeout != 4*time.Hour {
		t.Errorf("SessionTimeout = %v, want 4h", cfg.SessionTimeout)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `vault_path: /tmp/test-vault.json
session_timeout: 2h
log_level: debug
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.VaultPath != "/tmp/test-vault.json" {
		t.Errorf("VaultPath = %q, want /tmp/test-vault.json", cfg.VaultPath)
	}
	if cfg.SessionTimeout != 2*time.Hour {
		t.Errorf("SessionTimeout = %v, want 2h", cfg.SessionTimeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}
