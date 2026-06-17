package accounts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*http.Request)
		expect string
	}{
		{
			name:   "bearer authorization",
			setup:  func(r *http.Request) { r.Header.Set("Authorization", "Bearer sk-123") },
			expect: "sk-123",
		},
		{
			name:   "bearer case insensitive",
			setup:  func(r *http.Request) { r.Header.Set("Authorization", "bearer sk-xyz") },
			expect: "sk-xyz",
		},
		{
			name:   "raw authorization",
			setup:  func(r *http.Request) { r.Header.Set("Authorization", "sk-raw") },
			expect: "sk-raw",
		},
		{
			name:   "x-api-key",
			setup:  func(r *http.Request) { r.Header.Set("x-api-key", "sk-anthropic") },
			expect: "sk-anthropic",
		},
		{
			name:   "x-goog-api-key",
			setup:  func(r *http.Request) { r.Header.Set("x-goog-api-key", "sk-gemini") },
			expect: "sk-gemini",
		},
		{
			name:   "query key",
			setup:  func(r *http.Request) { r.URL.RawQuery = "key=sk-query" },
			expect: "sk-query",
		},
		{
			name:   "no key",
			setup:  func(r *http.Request) {},
			expect: "",
		},
		{
			name: "authorization wins over others",
			setup: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer sk-auth")
				r.Header.Set("x-api-key", "sk-other")
			},
			expect: "sk-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			tt.setup(r)
			if got := ExtractAPIKey(r); got != tt.expect {
				t.Fatalf("ExtractAPIKey = %q, want %q", got, tt.expect)
			}
		})
	}
}

func idHandler(id string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(id))
	})
}

func newTestAccount(id, key string) *Account {
	return &Account{
		ID:        id,
		APIKey:    key,
		OpenAI:    idHandler(id + ":openai"),
		Anthropic: idHandler(id + ":anthropic"),
		Gemini:    idHandler(id + ":gemini"),
		Usage:     idHandler(id + ":usage"),
	}
}

func TestRegistryDispatchByKey(t *testing.T) {
	reg, err := NewRegistry([]*Account{
		newTestAccount("alice", "key-alice"),
		newTestAccount("bob", "key-bob"),
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cases := []struct {
		proto Protocol
		key   string
		body  string
	}{
		{ProtoOpenAI, "key-alice", "alice:openai"},
		{ProtoAnthropic, "key-bob", "bob:anthropic"},
		{ProtoGemini, "key-alice", "alice:gemini"},
		{ProtoUsage, "key-bob", "bob:usage"},
	}

	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("Authorization", "Bearer "+c.key)
		w := httptest.NewRecorder()
		reg.Handler(c.proto).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("proto %d key %s: status = %d", c.proto, c.key, w.Code)
		}
		if w.Body.String() != c.body {
			t.Fatalf("proto %d key %s: body = %q, want %q", c.proto, c.key, w.Body.String(), c.body)
		}
	}
}

func TestRegistryRejectsUnknownAndMissingKey(t *testing.T) {
	reg, err := NewRegistry([]*Account{newTestAccount("alice", "key-alice")})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Unknown key.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer nope")
	w := httptest.NewRecorder()
	reg.Handler(ProtoOpenAI).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key: status = %d, want 401", w.Code)
	}

	// Missing key.
	r = httptest.NewRequest(http.MethodPost, "/", nil)
	w = httptest.NewRecorder()
	reg.Handler(ProtoOpenAI).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: status = %d, want 401", w.Code)
	}
}

func TestRegistryDuplicateKey(t *testing.T) {
	_, err := NewRegistry([]*Account{
		newTestAccount("alice", "dup"),
		newTestAccount("bob", "dup"),
	})
	if err == nil {
		t.Fatal("expected error for duplicate api key")
	}
}

func TestLegacyRegistryServesWithoutKey(t *testing.T) {
	reg := NewLegacyRegistry(newTestAccount("default", ""))

	r := httptest.NewRequest(http.MethodPost, "/", nil) // no key
	w := httptest.NewRecorder()
	reg.Handler(ProtoOpenAI).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy: status = %d, want 200", w.Code)
	}
	if w.Body.String() != "default:openai" {
		t.Fatalf("legacy: body = %q", w.Body.String())
	}
}

func TestLoadConfigMissingFileReturnsNil(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	content := `{"accounts":[
		{"id":"alice","api_key":"k1"},
		{"id":"bob","api_key":"k2","token_dir":"bob-dir"}
	]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.Accounts))
	}

	// token_dir defaults to id, resolved under base dir.
	if got := cfg.Accounts[0].ResolveTokenDir("/base"); got != filepath.Join("/base", "alice") {
		t.Fatalf("default token dir = %q", got)
	}
	if got := cfg.Accounts[1].ResolveTokenDir("/base"); got != filepath.Join("/base", "bob-dir") {
		t.Fatalf("custom token dir = %q", got)
	}
	// absolute token_dir is used as-is.
	abs := AccountConfig{ID: "x", APIKey: "k", TokenDir: "/abs/path"}
	if got := abs.ResolveTokenDir("/base"); got != "/abs/path" {
		t.Fatalf("absolute token dir = %q", got)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"missing id":    `{"accounts":[{"api_key":"k"}]}`,
		"missing key":   `{"accounts":[{"id":"a"}]}`,
		"duplicate id":  `{"accounts":[{"id":"a","api_key":"k1"},{"id":"a","api_key":"k2"}]}`,
		"duplicate key": `{"accounts":[{"id":"a","api_key":"k"},{"id":"b","api_key":"k"}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestResolveConfigPathEnvOverride(t *testing.T) {
	t.Setenv("COPILOT2API_ACCOUNTS_FILE", "/custom/accounts.json")
	if got := ResolveConfigPath("/base"); got != "/custom/accounts.json" {
		t.Fatalf("env override path = %q", got)
	}
	t.Setenv("COPILOT2API_ACCOUNTS_FILE", "")
	if got := ResolveConfigPath("/base"); got != filepath.Join("/base", DefaultConfigFileName) {
		t.Fatalf("default path = %q", got)
	}
}
