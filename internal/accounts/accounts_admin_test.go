package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/whtsky/copilot2api/auth"
	"github.com/whtsky/copilot2api/internal/stats"
)

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "accounts.json") // SaveConfig should create dirs
	in := &Config{Accounts: []AccountConfig{
		{ID: "alice", APIKey: "k1", TokenDir: "alice"},
		{ID: "bob", APIKey: "k2"},
	}}
	if err := SaveConfig(path, in); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	out, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(out.Accounts) != 2 || out.Accounts[0].ID != "alice" || out.Accounts[1].APIKey != "k2" {
		t.Fatalf("round trip mismatch: %+v", out.Accounts)
	}
}

func TestSaveConfigEmptyAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := SaveConfig(path, &Config{}); err != nil {
		t.Fatalf("SaveConfig empty: %v", err)
	}
	out, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if out == nil || len(out.Accounts) != 0 {
		t.Fatalf("expected empty config, got %+v", out)
	}
}

func TestSaveConfigRejectsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	err := SaveConfig(path, &Config{Accounts: []AccountConfig{
		{ID: "a", APIKey: "k"}, {ID: "b", APIKey: "k"},
	}})
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestRegistryMutations(t *testing.T) {
	reg, err := NewRegistry(nil) // empty bootstrap allowed
	if err != nil {
		t.Fatalf("NewRegistry(nil): %v", err)
	}
	if !reg.MultiAccount() {
		t.Fatal("expected multi-account mode")
	}

	if err := reg.Add(newTestAccount("alice", "k1")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Add(newTestAccount("alice", "k9")); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if err := reg.Add(newTestAccount("bob", "k1")); err == nil {
		t.Fatal("expected duplicate key error")
	}
	if err := reg.Add(newTestAccount("bob", "k2")); err != nil {
		t.Fatalf("Add bob: %v", err)
	}

	// Resolve by key.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer k2")
	if a, err := reg.Resolve(r); err != nil || a.ID != "bob" {
		t.Fatalf("Resolve k2 -> %v, %v", a, err)
	}

	// Rotate alice's key.
	if err := reg.UpdateKey("alice", "k2"); err == nil {
		t.Fatal("expected conflict rotating to bob's key")
	}
	if err := reg.UpdateKey("alice", "k1-new"); err != nil {
		t.Fatalf("UpdateKey: %v", err)
	}
	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	r2.Header.Set("x-api-key", "k1-new")
	if a, err := reg.Resolve(r2); err != nil || a.ID != "alice" {
		t.Fatalf("Resolve k1-new -> %v, %v", a, err)
	}
	// Old key no longer resolves.
	r3 := httptest.NewRequest(http.MethodPost, "/", nil)
	r3.Header.Set("x-api-key", "k1")
	if _, err := reg.Resolve(r3); err == nil {
		t.Fatal("expected old key to stop resolving")
	}

	// Remove bob.
	if _, err := reg.Remove("bob"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if reg.Get("bob") != nil {
		t.Fatal("bob should be gone")
	}
	if len(reg.Accounts()) != 1 {
		t.Fatalf("expected 1 account, got %d", len(reg.Accounts()))
	}
}

func TestLegacyRegistryRejectsAdd(t *testing.T) {
	reg := NewLegacyRegistry(newTestAccount("default", ""))
	if reg.MultiAccount() {
		t.Fatal("legacy should not be multi-account")
	}
	if err := reg.Add(newTestAccount("x", "k")); err == nil {
		t.Fatal("expected error adding to legacy registry")
	}
}

func TestRegistryConcurrentResolveAndMutate(t *testing.T) {
	reg, _ := NewRegistry([]*Account{newTestAccount("a", "ka")})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = reg.Add(newTestAccount("id"+strings.Repeat("x", 1), "k")) }()
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Header.Set("Authorization", "Bearer ka")
			_, _ = reg.Resolve(r)
		}()
	}
	wg.Wait()
}

func newManagerForTest(t *testing.T) (*Manager, string) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "accounts.json")
	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		return &Account{ID: c.ID, APIKey: c.APIKey, TokenDir: c.TokenDir, OpenAI: idHandler(c.ID)}, nil
	}
	return NewManager(reg, factory, cfgPath, "", nil), cfgPath
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestManagerCRUD(t *testing.T) {
	m, cfgPath := newManagerForTest(t)
	h := m.Handler()

	// Create.
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"alice","api_key":"k1"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	// Duplicate id -> 409.
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"alice","api_key":"k2"}`); w.Code != http.StatusConflict {
		t.Fatalf("dup create: %d", w.Code)
	}
	// Missing api_key -> auto-generated, should succeed.
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"x"}`); w.Code != http.StatusCreated {
		t.Fatalf("auto-gen key create: expected 201, got %d %s", w.Code, w.Body.String())
	}
	// Missing id -> 400.
	if w := do(h, "POST", "/admin/api/accounts", `{"api_key":"k9"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad create: expected 400, got %d", w.Code)
	}

	// Persisted to disk.
	cfg, err := LoadConfig(cfgPath)
	if err != nil || len(cfg.Accounts) != 2 || cfg.Accounts[0].ID != "alice" {
		t.Fatalf("config not persisted: %+v err=%v", cfg, err)
	}

	// List.
	w := do(h, "GET", "/admin/api/accounts", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var views []accountView
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	if len(views) != 2 || views[0].APIKey != "k1" {
		t.Fatalf("unexpected list: %+v", views)
	}

	// Rotate key.
	if w := do(h, "PUT", "/admin/api/accounts/alice", `{"api_key":"k1-new"}`); w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	if m.reg.Get("alice").APIKey != "k1-new" {
		t.Fatal("key not rotated in registry")
	}

	// Update missing account -> 404.
	if w := do(h, "PUT", "/admin/api/accounts/ghost", `{"api_key":"z"}`); w.Code != http.StatusNotFound {
		t.Fatalf("update ghost: %d", w.Code)
	}

	// Delete.
	if w := do(h, "DELETE", "/admin/api/accounts/alice", ""); w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	if m.reg.Get("alice") != nil {
		t.Fatal("alice still present")
	}
	// Also delete the auto-generated account.
	if w := do(h, "DELETE", "/admin/api/accounts/x", ""); w.Code != http.StatusOK {
		t.Fatalf("delete x: %d", w.Code)
	}
	cfg, _ = LoadConfig(cfgPath)
	if len(cfg.Accounts) != 0 {
		t.Fatalf("expected empty config after delete, got %+v", cfg.Accounts)
	}
}

func TestManagerAdminTokenGate(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "accounts.json")
	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		return &Account{ID: c.ID, APIKey: c.APIKey}, nil
	}
	m := NewManager(reg, factory, cfgPath, "secret", nil)
	h := m.Handler()

	// No token -> 401.
	if w := do(h, "GET", "/admin/api/accounts", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	// With token header -> 200.
	r := httptest.NewRequest("GET", "/admin/api/accounts", nil)
	r.Header.Set("X-Admin-Token", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("with token: %d", w.Code)
	}
	// With token query param -> 200.
	if w := do(h, "GET", "/admin/api/accounts?admin_token=secret", ""); w.Code != http.StatusOK {
		t.Fatalf("query token: %d", w.Code)
	}
}

func TestManagerServesIndex(t *testing.T) {
	m, _ := newManagerForTest(t)
	w := do(m.Handler(), "GET", "/admin/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("index: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("index content-type: %s", ct)
	}
	if !strings.Contains(w.Body.String(), "API Key") {
		t.Fatal("index body missing expected content")
	}
}

func TestManagerTokensEndpoint(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "accounts.json")

	// Seed a credentials.json so the account has a stored GitHub token.
	tokenDir := filepath.Join(dir, "alice")
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		t.Fatal(err)
	}
	creds := `{"github_token":"gho_secret123","copilot_token":{"token":"tid=x;exp=1","base_url":"https://api.example.com"}}`
	if err := os.WriteFile(filepath.Join(tokenDir, "credentials.json"), []byte(creds), 0600); err != nil {
		t.Fatal(err)
	}

	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		ac, err := auth.NewClient(tokenDir)
		if err != nil {
			return nil, err
		}
		return &Account{ID: c.ID, APIKey: c.APIKey, TokenDir: c.TokenDir, Auth: ac, OpenAI: idHandler(c.ID)}, nil
	}
	m := NewManager(reg, factory, cfgPath, "", nil)
	h := m.Handler()

	if w := do(h, "POST", "/admin/api/accounts", `{"id":"alice","api_key":"k1"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	w := do(h, "GET", "/admin/api/accounts/alice/tokens", "")
	if w.Code != http.StatusOK {
		t.Fatalf("tokens: %d %s", w.Code, w.Body.String())
	}
	var tok map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &tok); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tok["github_token"] != "gho_secret123" {
		t.Fatalf("github_token = %v", tok["github_token"])
	}
	if tok["copilot_token"] != "tid=x;exp=1" {
		t.Fatalf("copilot_token = %v", tok["copilot_token"])
	}

	// Unknown account -> 404.
	if w := do(h, "GET", "/admin/api/accounts/ghost/tokens", ""); w.Code != http.StatusNotFound {
		t.Fatalf("ghost tokens: %d", w.Code)
	}
}

func TestManagerStatsEndpoint(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "accounts.json")
	statsPath := filepath.Join(t.TempDir(), "stats.json")
	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		return &Account{ID: c.ID, APIKey: c.APIKey}, nil
	}
	store := stats.NewStore(statsPath)
	m := NewManager(reg, factory, cfgPath, "", store)
	h := m.Handler()

	// Record usage for two accounts/models.
	store.Recorder("alice").Record("gpt-5", stats.Usage{Input: 100, Output: 20, Cached: 30})
	store.Recorder("alice").Record("gpt-5", stats.Usage{Input: 10, Output: 5})
	store.Recorder("bob").Record("claude", stats.Usage{Input: 1, Output: 1, CacheCreation: 7})

	// GET /admin/api/stats returns per-account aggregates.
	w := do(h, "GET", "/admin/api/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", w.Code, w.Body.String())
	}
	var got []stats.AccountStats
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].ID != "alice" || got[1].ID != "bob" {
		t.Fatalf("unexpected accounts (want sorted alice,bob): %+v", got)
	}
	if got[0].Totals.Requests != 2 || got[0].Totals.Input != 110 || got[0].Totals.Cached != 30 {
		t.Fatalf("alice totals wrong: %+v", got[0].Totals)
	}
	if len(got[0].Models) != 1 || got[0].Models[0].Total != 165 {
		t.Fatalf("alice model total want 165: %+v", got[0].Models)
	}

	// Reset one account.
	if w := do(h, "DELETE", "/admin/api/stats/alice", ""); w.Code != http.StatusOK {
		t.Fatalf("reset: %d", w.Code)
	}
	w = do(h, "GET", "/admin/api/stats", "")
	got = nil
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "bob" {
		t.Fatalf("after reset want only bob: %+v", got)
	}
}
