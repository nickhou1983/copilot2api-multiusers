package accounts

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/whtsky/copilot2api/internal/stats"
)

//go:embed web/index.html
var adminIndexHTML []byte

// AccountFactory builds an unauthenticated Account (auth client + handlers) from
// a config entry. Authentication is performed separately via the device flow,
// so newly added accounts can be created before the user authorizes them.
type AccountFactory func(cfg AccountConfig) (*Account, error)

// Manager exposes an HTTP admin API + UI to maintain the API key ↔ GitHub
// account mapping. It keeps the live Registry and accounts.json in sync.
type Manager struct {
	mu         sync.Mutex
	reg        *Registry
	factory    AccountFactory
	cfgPath    string
	adminToken string
	stats      *stats.Store

	sessMu   sync.Mutex
	sessions map[string]*deviceSession
}

// NewManager creates an admin manager bound to a multi-account registry.
func NewManager(reg *Registry, factory AccountFactory, cfgPath, adminToken string, statsStore *stats.Store) *Manager {
	return &Manager{
		reg:        reg,
		factory:    factory,
		cfgPath:    cfgPath,
		adminToken: adminToken,
		stats:      statsStore,
		sessions:   make(map[string]*deviceSession),
	}
}

type deviceSession struct {
	mu        sync.Mutex
	userCode  string
	verifyURI string
	done      bool
	err       error
}

type accountView struct {
	ID            string `json:"id"`
	APIKey        string `json:"api_key"`
	TokenDir      string `json:"token_dir"`
	Authenticated bool   `json:"authenticated"`
	BaseURL       string `json:"base_url,omitempty"`
}

// Handler returns the admin HTTP handler tree rooted at /admin/.
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/{$}", m.handleIndex)
	mux.HandleFunc("GET /admin/api/accounts", m.handleList)
	mux.HandleFunc("POST /admin/api/accounts", m.handleCreate)
	mux.HandleFunc("PUT /admin/api/accounts/{id}", m.handleUpdate)
	mux.HandleFunc("DELETE /admin/api/accounts/{id}", m.handleDelete)
	mux.HandleFunc("POST /admin/api/accounts/{id}/auth/start", m.handleAuthStart)
	mux.HandleFunc("GET /admin/api/accounts/{id}/auth/status", m.handleAuthStatus)
	mux.HandleFunc("GET /admin/api/accounts/{id}/tokens", m.handleTokens)
	mux.HandleFunc("GET /admin/api/accounts/{id}/models", m.handleModels)
	mux.HandleFunc("GET /admin/api/generate-key", m.handleGenerateKey)
	mux.HandleFunc("GET /admin/api/stats", m.handleStats)
	mux.HandleFunc("DELETE /admin/api/stats", m.handleStatsResetAll)
	mux.HandleFunc("DELETE /admin/api/stats/{id}", m.handleStatsReset)
	return m.withAuth(mux)
}

// withAuth optionally gates admin requests behind COPILOT2API_ADMIN_TOKEN.
func (m *Manager) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.adminToken != "" {
			provided := r.Header.Get("X-Admin-Token")
			if provided == "" {
				provided = r.URL.Query().Get("admin_token")
			}
			if provided != m.adminToken {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid admin token"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(adminIndexHTML)
}

func (m *Manager) handleList(w http.ResponseWriter, _ *http.Request) {
	accs := m.reg.Accounts()
	views := make([]accountView, 0, len(accs))
	for _, a := range accs {
		views = append(views, m.viewOf(a))
	}
	sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	writeJSON(w, http.StatusOK, views)
}

func (m *Manager) viewOf(a *Account) accountView {
	v := accountView{ID: a.ID, APIKey: a.APIKey, TokenDir: a.TokenDir}
	if a.Auth != nil {
		v.Authenticated = a.Auth.IsAuthenticated()
		v.BaseURL = a.Auth.GetBaseURL()
	}
	return v
}

type createRequest struct {
	ID       string `json:"id"`
	APIKey   string `json:"api_key"`
	TokenDir string `json:"token_dir"`
}

func (m *Manager) handleGenerateKey(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"api_key": GenerateAPIKey()})
}

func (m *Manager) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.APIKey == "" {
		req.APIKey = GenerateAPIKey()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	acct, err := m.factory(AccountConfig{ID: req.ID, APIKey: req.APIKey, TokenDir: req.TokenDir})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := m.reg.Add(acct); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := m.persistLocked(); err != nil {
		_, _ = m.reg.Remove(acct.ID) // roll back on persistence failure
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m.viewOf(acct))
}

type updateRequest struct {
	APIKey   *string `json:"api_key"`
	TokenDir *string `json:"token_dir"`
}

func (m *Manager) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing := m.reg.Get(id)
	if existing == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}

	// A token_dir change requires rebuilding the account (new auth client).
	if req.TokenDir != nil && *req.TokenDir != existing.TokenDir {
		apiKey := existing.APIKey
		if req.APIKey != nil && *req.APIKey != "" {
			apiKey = *req.APIKey
		}
		rebuilt, err := m.factory(AccountConfig{ID: id, APIKey: apiKey, TokenDir: *req.TokenDir})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := m.reg.Replace(rebuilt); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if err := m.persistLocked(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, m.viewOf(rebuilt))
		return
	}

	if req.APIKey != nil && *req.APIKey != existing.APIKey {
		if *req.APIKey == "" {
			writeError(w, http.StatusBadRequest, "api_key cannot be empty")
			return
		}
		if err := m.reg.UpdateKey(id, *req.APIKey); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := m.persistLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m.viewOf(m.reg.Get(id)))
}

func (m *Manager) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := m.reg.Remove(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := m.persistLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

func (m *Manager) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil || acct.Auth == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}

	resp, err := acct.Auth.StartDeviceFlow()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	sess := &deviceSession{userCode: resp.UserCode, verifyURI: resp.VerificationURI}
	m.sessMu.Lock()
	m.sessions[id] = sess
	m.sessMu.Unlock()

	timeout := time.Duration(resp.ExpiresIn) * time.Second
	go func() {
		err := acct.Auth.CompleteDeviceFlow(resp.DeviceCode, resp.Interval, timeout)
		sess.mu.Lock()
		sess.done = true
		sess.err = err
		sess.mu.Unlock()
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"user_code":        resp.UserCode,
		"verification_uri": resp.VerificationURI,
		"expires_in":       resp.ExpiresIn,
		"interval":         resp.Interval,
	})
}

func (m *Manager) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil || acct.Auth == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}

	if acct.Auth.IsAuthenticated() {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "pending": false})
		return
	}

	m.sessMu.Lock()
	sess := m.sessions[id]
	m.sessMu.Unlock()

	resp := map[string]any{"authenticated": false, "pending": false}
	if sess != nil {
		sess.mu.Lock()
		resp["user_code"] = sess.userCode
		resp["verification_uri"] = sess.verifyURI
		resp["pending"] = !sess.done
		if sess.err != nil {
			resp["error"] = sess.err.Error()
		}
		sess.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (m *Manager) handleTokens(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil || acct.Auth == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}
	info := acct.Auth.StoredTokens()
	writeJSON(w, http.StatusOK, map[string]any{
		"github_token":       info.GitHubToken,
		"copilot_token":      info.CopilotToken,
		"copilot_expires_at": info.CopilotExpiresAt,
		"copilot_usable":     info.CopilotUsable,
		"base_url":           info.BaseURL,
	})
}

// handleModels proxies the account's cached upstream /models response so the
// admin UI can list the models GitHub Copilot supports.
func (m *Manager) handleModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}
	if acct.Models == nil {
		writeError(w, http.StatusServiceUnavailable, "models not available for account: "+id)
		return
	}
	if acct.Auth != nil && !acct.Auth.IsAuthenticated() {
		writeError(w, http.StatusConflict, "account not authenticated: "+id)
		return
	}
	raw, err := acct.Models.GetRaw(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch models: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (m *Manager) handleStats(w http.ResponseWriter, _ *http.Request) {
	if m.stats == nil {
		writeJSON(w, http.StatusOK, []stats.AccountStats{})
		return
	}
	writeJSON(w, http.StatusOK, m.stats.Snapshot())
}

func (m *Manager) handleStatsReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if m.stats == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "reset", "id": id})
		return
	}
	if err := m.stats.Reset(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset stats: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset", "id": id})
}

func (m *Manager) handleStatsResetAll(w http.ResponseWriter, _ *http.Request) {
	if m.stats == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
		return
	}
	if err := m.stats.ResetAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset stats: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// persistLocked writes the current registry state to accounts.json. Caller must
// hold m.mu.
func (m *Manager) persistLocked() error {
	accs := m.reg.Accounts()
	cfg := &Config{Accounts: make([]AccountConfig, 0, len(accs))}
	for _, a := range accs {
		cfg.Accounts = append(cfg.Accounts, AccountConfig{ID: a.ID, APIKey: a.APIKey, TokenDir: a.TokenDir})
	}
	sort.Slice(cfg.Accounts, func(i, j int) bool { return cfg.Accounts[i].ID < cfg.Accounts[j].ID })
	return SaveConfig(m.cfgPath, cfg)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
