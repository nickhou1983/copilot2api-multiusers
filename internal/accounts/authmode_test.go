package accounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAuthMode(t *testing.T) {
	t.Run("explicit direct", func(t *testing.T) {
		a := AccountConfig{ID: "a", APIKey: "k", AuthMode: AuthModeDirect}
		if got := a.ResolveAuthMode(); got != AuthModeDirect {
			t.Errorf("ResolveAuthMode = %q, want direct", got)
		}
	})

	t.Run("empty defaults to exchange", func(t *testing.T) {
		os.Unsetenv("COPILOT2API_AUTH_MODE")
		a := AccountConfig{ID: "a", APIKey: "k"}
		if got := a.ResolveAuthMode(); got != AuthModeExchange {
			t.Errorf("ResolveAuthMode = %q, want exchange", got)
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("COPILOT2API_AUTH_MODE", AuthModeDirect)
		a := AccountConfig{ID: "a", APIKey: "k"}
		if got := a.ResolveAuthMode(); got != AuthModeDirect {
			t.Errorf("ResolveAuthMode with env = %q, want direct", got)
		}
	})

	t.Run("explicit overrides env", func(t *testing.T) {
		t.Setenv("COPILOT2API_AUTH_MODE", AuthModeDirect)
		a := AccountConfig{ID: "a", APIKey: "k", AuthMode: AuthModeExchange}
		if got := a.ResolveAuthMode(); got != AuthModeExchange {
			t.Errorf("ResolveAuthMode = %q, want exchange", got)
		}
	})
}

func TestLoadConfigInvalidAuthMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	content := `{"accounts":[{"id":"a","api_key":"k","auth_mode":"bogus"}]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid auth_mode")
	}
}

func TestLoadConfigValidAuthMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	content := `{"accounts":[
		{"id":"a","api_key":"k1","auth_mode":"direct","enterprise_url":"company.ghe.com"},
		{"id":"b","api_key":"k2","auth_mode":"exchange"}
	]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Accounts[0].AuthMode != AuthModeDirect || cfg.Accounts[0].EnterpriseURL != "company.ghe.com" {
		t.Errorf("account a not parsed: %+v", cfg.Accounts[0])
	}
	if cfg.Accounts[1].ResolveAuthMode() != AuthModeExchange {
		t.Errorf("account b mode = %q", cfg.Accounts[1].ResolveAuthMode())
	}
}
