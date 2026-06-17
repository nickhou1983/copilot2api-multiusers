package accounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultConfigFileName is the name of the multi-account config file looked up
// inside the base token directory.
const DefaultConfigFileName = "accounts.json"

// AccountConfig describes a single API-key ↔ GitHub-account mapping.
type AccountConfig struct {
	// ID is a stable, unique identifier for the account. Used for logging and
	// as the default token sub-directory name.
	ID string `json:"id"`
	// APIKey is the bearer/API key clients must present to use this account.
	APIKey string `json:"api_key"`
	// TokenDir is where this account's credentials.json is stored. If relative,
	// it is resolved under the base token directory. Defaults to ID.
	TokenDir string `json:"token_dir,omitempty"`
}

// Config is the parsed accounts.json file.
type Config struct {
	Accounts []AccountConfig `json:"accounts"`
}

// ResolveConfigPath returns the accounts config file path. It honors the
// COPILOT2API_ACCOUNTS_FILE environment variable, otherwise falls back to
// <baseTokenDir>/accounts.json.
func ResolveConfigPath(baseTokenDir string) string {
	if v := os.Getenv("COPILOT2API_ACCOUNTS_FILE"); v != "" {
		return v
	}
	return filepath.Join(baseTokenDir, DefaultConfigFileName)
}

// LoadConfig reads and validates the accounts config at path. If the file does
// not exist it returns (nil, nil) so callers can fall back to legacy
// single-account mode.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read accounts config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse accounts config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	seenID := make(map[string]struct{}, len(c.Accounts))
	seenKey := make(map[string]struct{}, len(c.Accounts))
	for i := range c.Accounts {
		a := &c.Accounts[i]
		if a.ID == "" {
			return fmt.Errorf("accounts[%d]: id is required", i)
		}
		if a.APIKey == "" {
			return fmt.Errorf("account %q: api_key is required", a.ID)
		}
		if _, dup := seenID[a.ID]; dup {
			return fmt.Errorf("duplicate account id %q", a.ID)
		}
		if _, dup := seenKey[a.APIKey]; dup {
			return fmt.Errorf("duplicate api_key for account %q", a.ID)
		}
		seenID[a.ID] = struct{}{}
		seenKey[a.APIKey] = struct{}{}
	}
	return nil
}

// ResolveTokenDir returns the absolute token directory for an account, relative
// to baseTokenDir when AccountConfig.TokenDir is not absolute. An empty TokenDir
// defaults to the account ID.
func (a AccountConfig) ResolveTokenDir(baseTokenDir string) string {
	dir := a.TokenDir
	if dir == "" {
		dir = a.ID
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(baseTokenDir, dir)
}

// SaveConfig writes the accounts config to path atomically (temp file + rename).
func SaveConfig(path string, cfg *Config) error {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Accounts == nil {
		cfg.Accounts = []AccountConfig{}
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal accounts config: %w", err)
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write accounts config temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename accounts config file: %w", err)
	}
	return nil
}
