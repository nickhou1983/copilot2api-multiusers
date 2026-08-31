package accounts

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/whtsky/copilot2api/auth"
	"github.com/whtsky/copilot2api/internal/loginagent"
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

	// logins stores per-account GitHub credentials used for automated device
	// authorization. May be nil, which disables the login endpoints.
	logins *LoginStore
	// agent drives the browser container that completes the device flow. Nil
	// when COPILOT2API_LOGIN_AGENT_URL is unset, which keeps the manual flow
	// as the only option.
	agent *loginagent.Client

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

// WithAutoLogin attaches the GitHub credential store and the login-agent client
// that back automated device-flow authorization. Passing a nil agent leaves
// automation disabled while still allowing credentials to be stored.
func (m *Manager) WithAutoLogin(logins *LoginStore, agent *loginagent.Client) *Manager {
	m.logins = logins
	m.agent = agent
	return m
}

// autoLoginEnabled reports whether an automated authorization can be attempted.
func (m *Manager) autoLoginEnabled() bool {
	return m.logins != nil && m.agent != nil
}

// Automation states reported by the auth status endpoint.
const (
	autoStateIdle      = "idle"
	autoStateRunning   = "running"
	autoStateSucceeded = "succeeded"
	autoStateFailed    = "failed"
)

type deviceSession struct {
	mu        sync.Mutex
	userCode  string
	verifyURI string
	done      bool
	err       error

	// Automated-authorization progress. The device-flow poller above is
	// independent of these, so a failed automation always leaves the manual
	// path (user code + verification URI) usable.
	autoState  string
	autoStep   string
	autoClass  string
	autoErr    string
	screenshot []byte
}

// setAuto records the outcome of an automated authorization attempt.
func (s *deviceSession) setAuto(state, step, class, errMsg string, screenshot []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoState = state
	s.autoStep = step
	s.autoClass = class
	s.autoErr = errMsg
	if screenshot != nil {
		s.screenshot = screenshot
	}
}

type accountView struct {
	ID            string `json:"id"`
	APIKey        string `json:"api_key"`
	TokenDir      string `json:"token_dir"`
	AuthMode      string `json:"auth_mode,omitempty"`
	Authenticated bool   `json:"authenticated"`
	BaseURL       string `json:"base_url,omitempty"`
	// HasLogin reports whether GitHub credentials are stored for this account,
	// i.e. whether automated authorization can be attempted.
	HasLogin bool `json:"has_login"`
	// GitHubUsername is the stored login name. The password is never exposed.
	GitHubUsername string `json:"github_username,omitempty"`
}

// Handler returns the admin HTTP handler tree rooted at /admin/.
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/{$}", m.handleIndex)
	mux.HandleFunc("GET /admin/api/config", m.handleConfig)
	mux.HandleFunc("GET /admin/api/accounts", m.handleList)
	mux.HandleFunc("POST /admin/api/accounts", m.handleCreate)
	mux.HandleFunc("PUT /admin/api/accounts/{id}", m.handleUpdate)
	mux.HandleFunc("DELETE /admin/api/accounts/{id}", m.handleDelete)
	mux.HandleFunc("GET /admin/api/accounts/{id}/login", m.handleLoginGet)
	mux.HandleFunc("PUT /admin/api/accounts/{id}/login", m.handleLoginSet)
	mux.HandleFunc("DELETE /admin/api/accounts/{id}/login", m.handleLoginDelete)
	mux.HandleFunc("POST /admin/api/accounts/{id}/auth/start", m.handleAuthStart)
	mux.HandleFunc("GET /admin/api/accounts/{id}/auth/status", m.handleAuthStatus)
	mux.HandleFunc("GET /admin/api/accounts/{id}/auth/screenshot", m.handleAuthScreenshot)
	mux.HandleFunc("GET /admin/api/accounts/{id}/tokens", m.handleTokens)
	mux.HandleFunc("GET /admin/api/accounts/{id}/models", m.handleModels)
	mux.HandleFunc("POST /admin/api/accounts/{id}/models/refresh", m.handleModelsRefresh)
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
	v := accountView{ID: a.ID, APIKey: a.APIKey, TokenDir: a.TokenDir, AuthMode: a.AuthMode}
	if a.Auth != nil {
		v.Authenticated = a.Auth.IsAuthenticated()
		v.BaseURL = a.Auth.GetBaseURL()
	}
	if m.logins != nil {
		login := m.logins.View(a.ID)
		v.GitHubUsername = login.Username
		v.HasLogin = m.logins.Has(a.ID)
	}
	return v
}

// handleConfig reports admin-UI feature availability so the front end can hide
// controls that the deployment has not enabled.
func (m *Manager) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auto_login_enabled":  m.autoLoginEnabled(),
		"login_store_enabled": m.logins != nil,
		"login_agent_url":     m.agent.BaseURL(),
	})
}

type createRequest struct {
	ID       string `json:"id"`
	APIKey   string `json:"api_key"`
	TokenDir string `json:"token_dir"`
	AuthMode string `json:"auth_mode"`
	// Optional GitHub credentials, so an account and its automated-login
	// credentials can be created in one step.
	GitHubUsername string `json:"github_username"`
	GitHubPassword string `json:"github_password"`
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
	if _, err := auth.ParseMode(req.AuthMode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Validate credentials before touching the registry so a bad pair does not
	// leave a half-configured account behind.
	wantsLogin := req.GitHubUsername != "" || req.GitHubPassword != ""
	if wantsLogin {
		if m.logins == nil {
			writeError(w, http.StatusServiceUnavailable, "GitHub credential storage is not enabled")
			return
		}
		if req.GitHubUsername == "" || req.GitHubPassword == "" {
			writeError(w, http.StatusBadRequest, "github_username and github_password must be provided together")
			return
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	acct, err := m.factory(AccountConfig{ID: req.ID, APIKey: req.APIKey, TokenDir: req.TokenDir, AuthMode: req.AuthMode})
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
	if wantsLogin {
		login := GitHubLogin{Username: req.GitHubUsername, Password: req.GitHubPassword}
		if err := m.logins.Set(acct.ID, login); err != nil {
			_, _ = m.reg.Remove(acct.ID)
			_ = m.persistLocked()
			writeError(w, http.StatusInternalServerError, "failed to save github credentials: "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, m.viewOf(acct))
}

type updateRequest struct {
	APIKey   *string `json:"api_key"`
	TokenDir *string `json:"token_dir"`
	AuthMode *string `json:"auth_mode"`
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

	if req.AuthMode != nil {
		if _, err := auth.ParseMode(*req.AuthMode); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// A token_dir or auth_mode change requires rebuilding the account (new auth client).
	tokenDirChanged := req.TokenDir != nil && *req.TokenDir != existing.TokenDir
	authModeChanged := req.AuthMode != nil && *req.AuthMode != existing.AuthMode
	if tokenDirChanged || authModeChanged {
		apiKey := existing.APIKey
		if req.APIKey != nil && *req.APIKey != "" {
			apiKey = *req.APIKey
		}
		tokenDir := existing.TokenDir
		if req.TokenDir != nil {
			tokenDir = *req.TokenDir
		}
		authMode := existing.AuthMode
		if req.AuthMode != nil {
			authMode = *req.AuthMode
		}
		rebuilt, err := m.factory(AccountConfig{ID: id, APIKey: apiKey, TokenDir: tokenDir, AuthMode: authMode})
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
	// Cascade: stored GitHub credentials outlive nothing, so drop them with the
	// account rather than leaving orphaned secrets on disk.
	if m.logins != nil {
		if err := m.logins.Delete(id); err != nil {
			slog.Warn("failed to delete github credentials", "account", id, "error", err)
		}
	}
	m.sessMu.Lock()
	delete(m.sessions, id)
	m.sessMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginAccount resolves the account for the credential endpoints, writing the
// error response and returning ok=false when unavailable.
func (m *Manager) loginAccount(w http.ResponseWriter, r *http.Request) (string, bool) {
	if m.logins == nil {
		writeError(w, http.StatusServiceUnavailable, "GitHub credential storage is not enabled")
		return "", false
	}
	id := r.PathValue("id")
	if m.reg.Get(id) == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return "", false
	}
	return id, true
}

// handleLoginGet returns the redacted GitHub credentials for an account. The
// password is never returned.
func (m *Manager) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	id, ok := m.loginAccount(w, r)
	if !ok {
		return
	}
	view := m.logins.View(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":     view.Username,
		"has_password": view.HasPassword,
	})
}

// handleLoginSet stores the GitHub credentials used for automated device-flow
// authorization.
func (m *Manager) handleLoginSet(w http.ResponseWriter, r *http.Request) {
	id, ok := m.loginAccount(w, r)
	if !ok {
		return
	}
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := m.logins.Set(id, GitHubLogin{Username: req.Username, Password: req.Password}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view := m.logins.View(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":     view.Username,
		"has_password": view.HasPassword,
	})
}

// handleLoginDelete removes an account's stored GitHub credentials.
func (m *Manager) handleLoginDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := m.loginAccount(w, r)
	if !ok {
		return
	}
	if err := m.logins.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete github credentials: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

type authStartRequest struct {
	// Auto asks the login agent to complete the authorization in the browser
	// container instead of waiting for a human.
	Auto bool `json:"auto"`
}

func (m *Manager) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil || acct.Auth == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}

	// The body is optional: an empty request keeps the original manual flow.
	var req authStartRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	var login GitHubLogin
	if req.Auto {
		if !m.autoLoginEnabled() {
			writeError(w, http.StatusServiceUnavailable, "automated login is not enabled (set "+loginagent.EnvURL+")")
			return
		}
		stored, ok := m.logins.Get(id)
		if !ok || stored.Username == "" || stored.Password == "" {
			writeError(w, http.StatusConflict, "no GitHub credentials stored for account: "+id)
			return
		}
		login = stored
	}

	resp, err := acct.Auth.StartDeviceFlow()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	sess := &deviceSession{userCode: resp.UserCode, verifyURI: resp.VerificationURI, autoState: autoStateIdle}
	m.sessMu.Lock()
	m.sessions[id] = sess
	m.sessMu.Unlock()

	timeout := time.Duration(resp.ExpiresIn) * time.Second
	// The poller is intentionally independent of the automation: if the browser
	// agent fails, the user code stays valid and an operator can still
	// authorize by hand within the same window.
	go func() {
		err := acct.Auth.CompleteDeviceFlow(resp.DeviceCode, resp.Interval, timeout)
		sess.mu.Lock()
		sess.done = true
		sess.err = err
		sess.mu.Unlock()
	}()

	if req.Auto {
		sess.setAuto(autoStateRunning, "", "", "", nil)
		go m.runAutoLogin(id, login, resp.UserCode, resp.VerificationURI, timeout, sess)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_code":        resp.UserCode,
		"verification_uri": resp.VerificationURI,
		"expires_in":       resp.ExpiresIn,
		"interval":         resp.Interval,
		"auto":             req.Auto,
	})
}

// runAutoLogin drives the browser container through the device authorization
// and records the outcome on the session for the status endpoint.
func (m *Manager) runAutoLogin(id string, login GitHubLogin, userCode, verifyURI string, timeout time.Duration, sess *deviceSession) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("starting automated device authorization", "account", id, "agent", m.agent.BaseURL())
	result, err := m.agent.Login(ctx, loginagent.LoginRequest{
		AccountID:       id,
		Username:        login.Username,
		Password:        login.Password,
		UserCode:        userCode,
		VerificationURI: verifyURI,
	})
	if err != nil {
		slog.Warn("automated device authorization failed", "account", id, "error", err)
		sess.setAuto(autoStateFailed, "", loginagent.ErrUnknown, err.Error(), nil)
		return
	}
	if result.OK {
		slog.Info("automated device authorization succeeded", "account", id)
		sess.setAuto(autoStateSucceeded, result.Step, "", "", nil)
		return
	}
	slog.Warn("automated device authorization failed",
		"account", id, "step", result.Step, "class", result.ErrorClass, "error", result.Error)
	sess.setAuto(autoStateFailed, result.Step, result.ErrorClass, result.Error, result.ScreenshotPNG)
}

func (m *Manager) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil || acct.Auth == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return
	}

	m.sessMu.Lock()
	sess := m.sessions[id]
	m.sessMu.Unlock()

	// Always report the automation state, even once authenticated, so the UI
	// can show how the account was authorized.
	autoInfo := map[string]any{"state": autoStateIdle}
	if sess != nil {
		sess.mu.Lock()
		if sess.autoState != "" {
			autoInfo["state"] = sess.autoState
		}
		autoInfo["step"] = sess.autoStep
		autoInfo["error_class"] = sess.autoClass
		autoInfo["error"] = sess.autoErr
		autoInfo["has_screenshot"] = len(sess.screenshot) > 0
		sess.mu.Unlock()
	}

	if acct.Auth.IsAuthenticated() {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "pending": false, "auto": autoInfo})
		return
	}

	resp := map[string]any{"authenticated": false, "pending": false, "auto": autoInfo}
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

// handleAuthScreenshot serves the screenshot captured when an automated login
// failed, so an operator can see which page the browser got stuck on.
func (m *Manager) handleAuthScreenshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	m.sessMu.Lock()
	sess := m.sessions[id]
	m.sessMu.Unlock()

	var png []byte
	if sess != nil {
		sess.mu.Lock()
		png = sess.screenshot
		sess.mu.Unlock()
	}
	if len(png) == 0 {
		writeError(w, http.StatusNotFound, "no screenshot available for account: "+id)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
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
	acct, ok := m.modelsAccount(w, r)
	if !ok {
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

// handleModelsRefresh forces a fresh upstream /models fetch, bypassing the
// cache TTL, so the admin UI can update the model list on demand.
func (m *Manager) handleModelsRefresh(w http.ResponseWriter, r *http.Request) {
	acct, ok := m.modelsAccount(w, r)
	if !ok {
		return
	}
	raw, err := acct.Models.Refresh(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to refresh models: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// modelsAccount resolves and validates the account for models endpoints,
// writing the error response and returning ok=false when unavailable.
func (m *Manager) modelsAccount(w http.ResponseWriter, r *http.Request) (*Account, bool) {
	id := r.PathValue("id")
	acct := m.reg.Get(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found: "+id)
		return nil, false
	}
	if acct.Models == nil {
		writeError(w, http.StatusServiceUnavailable, "models not available for account: "+id)
		return nil, false
	}
	if acct.Auth != nil && !acct.Auth.IsAuthenticated() {
		writeError(w, http.StatusConflict, "account not authenticated: "+id)
		return nil, false
	}
	return acct, true
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
		cfg.Accounts = append(cfg.Accounts, AccountConfig{ID: a.ID, APIKey: a.APIKey, TokenDir: a.TokenDir, AuthMode: a.AuthMode})
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
