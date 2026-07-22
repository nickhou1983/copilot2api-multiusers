package accounts

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/whtsky/copilot2api/auth"
)

func TestConfigValidateRejectsInvalidAuthMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	err := SaveConfig(path, &Config{Accounts: []AccountConfig{
		{ID: "a", APIKey: "k", AuthMode: "bogus"},
	}})
	if err == nil {
		t.Fatal("expected invalid auth mode error")
	}
}

func TestConfigAuthModeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	in := &Config{Accounts: []AccountConfig{
		{ID: "a", APIKey: "k1", AuthMode: "direct"},
		{ID: "b", APIKey: "k2"},
	}}
	if err := SaveConfig(path, in); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	out, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if out.Accounts[0].AuthMode != "direct" || out.Accounts[1].AuthMode != "" {
		t.Fatalf("auth_mode round trip mismatch: %+v", out.Accounts)
	}
}

func TestResolveAuthMode(t *testing.T) {
	t.Setenv("COPILOT2API_AUTH_MODE", "")

	if m, err := (AccountConfig{}).ResolveAuthMode(); err != nil || m != auth.ModeExchange {
		t.Fatalf("default mode = %v, %v; want exchange", m, err)
	}
	if m, err := (AccountConfig{AuthMode: "direct"}).ResolveAuthMode(); err != nil || m != auth.ModeDirect {
		t.Fatalf("account mode = %v, %v; want direct", m, err)
	}

	t.Setenv("COPILOT2API_AUTH_MODE", "direct")
	if m, err := (AccountConfig{}).ResolveAuthMode(); err != nil || m != auth.ModeDirect {
		t.Fatalf("env fallback mode = %v, %v; want direct", m, err)
	}
	// Account setting wins over env.
	if m, err := (AccountConfig{AuthMode: "exchange"}).ResolveAuthMode(); err != nil || m != auth.ModeExchange {
		t.Fatalf("account override = %v, %v; want exchange", m, err)
	}

	t.Setenv("COPILOT2API_AUTH_MODE", "bogus")
	if _, err := (AccountConfig{}).ResolveAuthMode(); err == nil {
		t.Fatal("expected error for invalid env mode")
	}
}

func TestAdminCreateAndUpdateAuthMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "accounts.json")

	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		ac, err := auth.NewClient(c.ResolveTokenDir(dir), auth.Mode(c.AuthMode))
		if err != nil {
			return nil, err
		}
		return &Account{ID: c.ID, APIKey: c.APIKey, TokenDir: c.TokenDir, AuthMode: c.AuthMode, Auth: ac}, nil
	}
	m := NewManager(reg, factory, cfgPath, "", nil)
	h := m.Handler()

	if w := do(h, "POST", "/admin/api/accounts", `{"id":"alice","api_key":"k1","auth_mode":"direct"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"bob","api_key":"k2","auth_mode":"bogus"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("create with bogus mode: %d %s", w.Code, w.Body.String())
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Accounts) != 1 || cfg.Accounts[0].AuthMode != "direct" {
		t.Fatalf("persisted config mismatch: %+v", cfg.Accounts)
	}

	// Switching auth_mode rebuilds the account and persists.
	if w := do(h, "PUT", "/admin/api/accounts/alice", `{"auth_mode":"exchange"}`); w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	cfg, err = LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Accounts[0].AuthMode != "exchange" {
		t.Fatalf("updated config mismatch: %+v", cfg.Accounts)
	}
	if got := reg.Get("alice").Auth.Mode(); got != auth.ModeExchange {
		t.Fatalf("rebuilt account mode = %q, want exchange", got)
	}
}
