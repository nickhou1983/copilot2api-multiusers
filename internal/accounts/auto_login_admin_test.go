package accounts

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whtsky/copilot2api/auth"
	"github.com/whtsky/copilot2api/internal/loginagent"
)

// autoTestEnv bundles a manager wired for automated login plus the fake agent
// backing it, so tests can assert on what the manager sent.
type autoTestEnv struct {
	m       *Manager
	h       http.Handler
	logins  *LoginStore
	agentRx chan loginagent.LoginRequest
}

// newAutoManager builds a manager with a credential store and, when agentBody
// is non-nil, a login agent whose /login endpoint returns that payload.
func newAutoManager(t *testing.T, agentBody map[string]any) *autoTestEnv {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "accounts.json")

	reg, _ := NewRegistry(nil)
	factory := func(c AccountConfig) (*Account, error) {
		// A real auth client is offline-safe: it only touches the token dir.
		ac, err := auth.NewClient(filepath.Join(dir, "tokens", c.ID), auth.ModeExchange)
		if err != nil {
			return nil, err
		}
		return &Account{ID: c.ID, APIKey: c.APIKey, TokenDir: c.TokenDir, Auth: ac, OpenAI: idHandler(c.ID)}, nil
	}

	logins, err := NewLoginStore(filepath.Join(dir, DefaultLoginsFileName))
	if err != nil {
		t.Fatalf("NewLoginStore: %v", err)
	}

	env := &autoTestEnv{logins: logins, agentRx: make(chan loginagent.LoginRequest, 4)}

	var client *loginagent.Client
	if agentBody != nil {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req loginagent.LoginRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			env.agentRx <- req
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(agentBody)
		}))
		t.Cleanup(srv.Close)
		client = loginagent.NewClient(srv.URL, "", 5*time.Second)
	}

	env.m = NewManager(reg, factory, cfgPath, "", nil).WithAutoLogin(logins, client)
	env.h = env.m.Handler()
	return env
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return got
}

func TestConfigEndpointReportsAutoLogin(t *testing.T) {
	// Disabled by default: no credential store, no agent.
	m, _ := newManagerForTest(t)
	got := decodeBody(t, do(m.Handler(), "GET", "/admin/api/config", ""))
	if got["auto_login_enabled"] != false || got["login_store_enabled"] != false {
		t.Fatalf("expected feature disabled, got %+v", got)
	}

	env := newAutoManager(t, map[string]any{"ok": true})
	got = decodeBody(t, do(env.h, "GET", "/admin/api/config", ""))
	if got["auto_login_enabled"] != true || got["login_store_enabled"] != true {
		t.Fatalf("expected feature enabled, got %+v", got)
	}
	if !strings.HasPrefix(got["login_agent_url"].(string), "http://") {
		t.Fatalf("expected agent url, got %+v", got["login_agent_url"])
	}
}

func TestLoginEndpointsDisabledWithoutStore(t *testing.T) {
	m, _ := newManagerForTest(t)
	h := m.Handler()
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"a","api_key":"k"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	for _, tc := range []struct{ method, body string }{
		{"GET", ""},
		{"PUT", `{"username":"u","password":"p"}`},
		{"DELETE", ""},
	} {
		if w := do(h, tc.method, "/admin/api/accounts/a/login", tc.body); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s login: expected 503, got %d", tc.method, w.Code)
		}
	}
	// Creating an account with credentials must fail loudly rather than
	// silently dropping the password.
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"b","api_key":"k2","github_username":"u","github_password":"p"}`); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("create with creds: expected 503, got %d", w.Code)
	}
}

func TestLoginCRUD(t *testing.T) {
	env := newAutoManager(t, nil)
	h := env.h

	if w := do(h, "PUT", "/admin/api/accounts/ghost/login", `{"username":"u","password":"p"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown account: expected 404, got %d", w.Code)
	}
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"alice","api_key":"k1"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	// Empty before it is set.
	got := decodeBody(t, do(h, "GET", "/admin/api/accounts/alice/login", ""))
	if got["username"] != "" || got["has_password"] != false {
		t.Fatalf("expected empty login, got %+v", got)
	}

	// Set.
	w := do(h, "PUT", "/admin/api/accounts/alice/login", `{"username":"alice@example.com","password":"hunter2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("set login: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "hunter2") {
		t.Fatalf("password leaked in response: %s", w.Body.String())
	}
	got = decodeBody(t, w)
	if got["username"] != "alice@example.com" || got["has_password"] != true {
		t.Fatalf("unexpected login view: %+v", got)
	}

	// Missing password -> 400.
	if w := do(h, "PUT", "/admin/api/accounts/alice/login", `{"username":"u","password":""}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty password: expected 400, got %d", w.Code)
	}

	// The account listing advertises the credential without exposing it.
	listBody := do(h, "GET", "/admin/api/accounts", "").Body.String()
	if !strings.Contains(listBody, `"has_login":true`) || !strings.Contains(listBody, "alice@example.com") {
		t.Fatalf("list missing login info: %s", listBody)
	}
	if strings.Contains(listBody, "hunter2") {
		t.Fatalf("password leaked in list: %s", listBody)
	}

	// Delete.
	if w := do(h, "DELETE", "/admin/api/accounts/alice/login", ""); w.Code != http.StatusOK {
		t.Fatalf("delete login: %d", w.Code)
	}
	got = decodeBody(t, do(h, "GET", "/admin/api/accounts/alice/login", ""))
	if got["has_password"] != false {
		t.Fatalf("expected credentials cleared, got %+v", got)
	}
}

func TestCreateAccountWithCredentials(t *testing.T) {
	env := newAutoManager(t, nil)
	h := env.h

	// Username without password is rejected before the account is created.
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"half","api_key":"k","github_username":"u"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("partial creds: expected 400, got %d", w.Code)
	}
	if env.m.reg.Get("half") != nil {
		t.Fatal("account should not exist after rejected create")
	}

	w := do(h, "POST", "/admin/api/accounts", `{"id":"bob","api_key":"k2","github_username":"bob@example.com","github_password":"s3cret"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Fatalf("password leaked: %s", w.Body.String())
	}
	got := decodeBody(t, w)
	if got["has_login"] != true || got["github_username"] != "bob@example.com" {
		t.Fatalf("unexpected view: %+v", got)
	}
	if login, ok := env.logins.Get("bob"); !ok || login.Password != "s3cret" {
		t.Fatalf("credentials not stored: %+v ok=%v", login, ok)
	}
}

func TestDeleteAccountCascadesLogin(t *testing.T) {
	env := newAutoManager(t, nil)
	h := env.h

	if w := do(h, "POST", "/admin/api/accounts", `{"id":"carol","api_key":"k","github_username":"c","github_password":"p"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if !env.logins.Has("carol") {
		t.Fatal("expected stored credentials")
	}
	if w := do(h, "DELETE", "/admin/api/accounts/carol", ""); w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	if env.logins.Has("carol") {
		t.Fatal("credentials should be removed with the account")
	}
	// Reloading from disk proves the cascade was persisted, not just in memory.
	reloaded, err := NewLoginStore(env.logins.path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Has("carol") {
		t.Fatal("credentials still on disk after account deletion")
	}
}

func TestAuthStartAutoRequiresAgent(t *testing.T) {
	// Store present, agent absent -> automation unavailable.
	env := newAutoManager(t, nil)
	h := env.h
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"dave","api_key":"k","github_username":"d","github_password":"p"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := do(h, "POST", "/admin/api/accounts/dave/auth/start", `{"auto":true}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without agent, got %d %s", w.Code, w.Body.String())
	}
}

func TestAuthStartAutoRequiresCredentials(t *testing.T) {
	env := newAutoManager(t, map[string]any{"ok": true})
	h := env.h
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"erin","api_key":"k"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	// Must fail before any device flow is started, so no network is touched.
	w := do(h, "POST", "/admin/api/accounts/erin/auth/start", `{"auto":true}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 without credentials, got %d %s", w.Code, w.Body.String())
	}
}

func TestAuthStartRejectsUnknownFields(t *testing.T) {
	env := newAutoManager(t, map[string]any{"ok": true})
	w := do(env.h, "POST", "/admin/api/accounts/nobody/auth/start", `{"auto":true,"bogus":1}`)
	// Unknown account is checked first; the point is that it never 500s.
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestAuthStatusReportsAutomation(t *testing.T) {
	env := newAutoManager(t, map[string]any{"ok": true})
	h := env.h
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"frank","api_key":"k"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	// No session yet: automation reports idle rather than omitting the field.
	got := decodeBody(t, do(h, "GET", "/admin/api/accounts/frank/auth/status", ""))
	auto, ok := got["auto"].(map[string]any)
	if !ok || auto["state"] != autoStateIdle {
		t.Fatalf("expected idle automation, got %+v", got["auto"])
	}

	// Seed a failed session directly: driving the real device flow would
	// require network access.
	png := []byte("\x89PNG\r\n\x1a\nfake")
	sess := &deviceSession{userCode: "ABCD-1234", verifyURI: "https://github.com/login/device"}
	sess.setAuto(autoStateFailed, "login", loginagent.ErrInvalidCredentials, "bad password", png)
	env.m.sessMu.Lock()
	env.m.sessions["frank"] = sess
	env.m.sessMu.Unlock()

	got = decodeBody(t, do(h, "GET", "/admin/api/accounts/frank/auth/status", ""))
	auto = got["auto"].(map[string]any)
	if auto["state"] != autoStateFailed || auto["step"] != "login" ||
		auto["error_class"] != loginagent.ErrInvalidCredentials || auto["has_screenshot"] != true {
		t.Fatalf("unexpected automation status: %+v", auto)
	}
	// The manual fallback must remain available after an automation failure.
	if got["user_code"] != "ABCD-1234" || got["verification_uri"] != "https://github.com/login/device" {
		t.Fatalf("manual fallback data missing: %+v", got)
	}
}

func TestAuthScreenshotEndpoint(t *testing.T) {
	env := newAutoManager(t, map[string]any{"ok": true})
	h := env.h
	if w := do(h, "POST", "/admin/api/accounts", `{"id":"grace","api_key":"k"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := do(h, "GET", "/admin/api/accounts/grace/auth/screenshot", ""); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without screenshot, got %d", w.Code)
	}

	png := []byte("\x89PNG\r\n\x1a\nfake-bytes")
	sess := &deviceSession{}
	sess.setAuto(autoStateFailed, "authorize", loginagent.ErrTimeout, "timed out", png)
	env.m.sessMu.Lock()
	env.m.sessions["grace"] = sess
	env.m.sessMu.Unlock()

	w := do(h, "GET", "/admin/api/accounts/grace/auth/screenshot", "")
	if w.Code != http.StatusOK {
		t.Fatalf("screenshot: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type: %q", ct)
	}
	if w.Body.String() != string(png) {
		t.Fatalf("screenshot bytes mismatch")
	}
}

func TestRunAutoLoginRecordsOutcome(t *testing.T) {
	shot := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nboom"))
	env := newAutoManager(t, map[string]any{
		"ok":          false,
		"step":        "device_code",
		"error_class": loginagent.ErrCodeRejected,
		"error":       "code was rejected",
		"screenshot":  shot,
	})

	sess := &deviceSession{}
	login := GitHubLogin{Username: "u@example.com", Password: "pw"}
	env.m.runAutoLogin("acct", login, "ABCD-1234", "https://github.com/login/device", 5*time.Second, sess)

	select {
	case req := <-env.agentRx:
		if req.AccountID != "acct" || req.Username != "u@example.com" ||
			req.Password != "pw" || req.UserCode != "ABCD-1234" {
			t.Fatalf("agent received unexpected request: %+v", req)
		}
	default:
		t.Fatal("agent was never called")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.autoState != autoStateFailed || sess.autoStep != "device_code" ||
		sess.autoClass != loginagent.ErrCodeRejected || sess.autoErr != "code was rejected" {
		t.Fatalf("unexpected session state: %+v", sess)
	}
	if string(sess.screenshot) != "\x89PNG\r\n\x1a\nboom" {
		t.Fatalf("screenshot not stored, got %q", sess.screenshot)
	}
	// A failed automation must never mark the device flow as finished; the
	// poller owns that flag so manual authorization still completes.
	if sess.done {
		t.Fatal("automation must not terminate the device-flow poller")
	}
}

func TestRunAutoLoginSuccess(t *testing.T) {
	env := newAutoManager(t, map[string]any{"ok": true, "step": "done"})
	sess := &deviceSession{}
	env.m.runAutoLogin("acct", GitHubLogin{Username: "u", Password: "p"},
		"ABCD-1234", "https://github.com/login/device", 5*time.Second, sess)

	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.autoState != autoStateSucceeded || sess.autoStep != "done" || sess.autoErr != "" {
		t.Fatalf("unexpected session state: %+v", sess)
	}
}
