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
	// Pool, when set, groups this account with other accounts that declare the
	// same pool name into an account pool. All members of a pool MUST share the
	// same api_key: requests to that key are distributed across pool members
	// with round-robin selection and automatic failover. Leaving Pool empty
	// makes the account standalone (a pool of one), preserving the classic
	// one-key-one-account behavior.
	Pool string `json:"pool,omitempty"`
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
	// keyPool tracks, per api_key, the pool name it is associated with and how
	// many accounts use it, so we can allow a shared key only within one pool.
	type keyInfo struct {
		pool  string
		count int
	}
	byKey := make(map[string]*keyInfo, len(c.Accounts))
	// poolKey ensures a pool name maps to exactly one api_key.
	poolKey := make(map[string]string, len(c.Accounts))
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
		seenID[a.ID] = struct{}{}

		if ki, ok := byKey[a.APIKey]; ok {
			// The key is already used. It may only be reused when both this and
			// the prior account(s) belong to the SAME non-empty pool.
			if a.Pool == "" || ki.pool == "" || a.Pool != ki.pool {
				return fmt.Errorf("duplicate api_key for account %q (share a key only within the same pool)", a.ID)
			}
			ki.count++
		} else {
			byKey[a.APIKey] = &keyInfo{pool: a.Pool, count: 1}
		}

		if a.Pool != "" {
			if k, ok := poolKey[a.Pool]; ok && k != a.APIKey {
				return fmt.Errorf("pool %q maps to more than one api_key", a.Pool)
			}
			poolKey[a.Pool] = a.APIKey
		}
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
