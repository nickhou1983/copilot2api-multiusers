package loginagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientDisabledWithoutURL(t *testing.T) {
	if c := NewClient("", "tok", 0); c != nil {
		t.Fatal("expected nil client for empty base URL")
	}
	if c := NewClient("   ", "tok", 0); c != nil {
		t.Fatal("expected nil client for blank base URL")
	}

	var nilClient *Client
	if nilClient.BaseURL() != "" || nilClient.Timeout() != 0 {
		t.Fatal("expected zero values from nil client")
	}
	if _, err := nilClient.Login(context.Background(), LoginRequest{}); err == nil {
		t.Fatal("expected error from nil client Login")
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://agent:8080/", "", 0)
	if c.BaseURL() != "http://agent:8080" {
		t.Fatalf("BaseURL = %q", c.BaseURL())
	}
	if c.Timeout() != DefaultTimeout {
		t.Fatalf("Timeout = %v, want %v", c.Timeout(), DefaultTimeout)
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv(EnvURL, "")
	if c := NewClientFromEnv(); c != nil {
		t.Fatal("expected nil client when agent URL is unset")
	}

	t.Setenv(EnvURL, "http://login-agent:8080")
	t.Setenv(EnvTimeout, "42")
	c := NewClientFromEnv()
	if c == nil {
		t.Fatal("expected client")
	}
	if c.Timeout() != 42*time.Second {
		t.Fatalf("Timeout = %v", c.Timeout())
	}

	t.Setenv(EnvTimeout, "not-a-number")
	if got := NewClientFromEnv().Timeout(); got != DefaultTimeout {
		t.Fatalf("expected default timeout for invalid env, got %v", got)
	}
}

func TestLoginSuccess(t *testing.T) {
	var gotReq LoginRequest
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotToken = r.Header.Get(TokenHeader)
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		writeAgentJSON(w, http.StatusOK, agentResponse{OK: true, Step: "done"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret", 5*time.Second)
	res, err := c.Login(context.Background(), LoginRequest{
		AccountID:       "alice",
		Username:        "alice-gh",
		Password:        "s3cret",
		UserCode:        "ABCD-1234",
		VerificationURI: "https://github.com/login/device",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !res.OK || res.Step != "done" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if gotToken != "shared-secret" {
		t.Fatalf("agent token header = %q", gotToken)
	}
	if gotReq.UserCode != "ABCD-1234" || gotReq.Username != "alice-gh" || gotReq.Password != "s3cret" {
		t.Fatalf("unexpected forwarded request: %+v", gotReq)
	}
}

func TestLoginFailureWithScreenshot(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAgentJSON(w, http.StatusOK, agentResponse{
			OK:         false,
			Step:       "login",
			ErrorClass: ErrInvalidCredentials,
			Error:      "incorrect username or password",
			Screenshot: base64.StdEncoding.EncodeToString(png),
		})
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "", 5*time.Second).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.OK {
		t.Fatal("expected failure")
	}
	if res.ErrorClass != ErrInvalidCredentials {
		t.Fatalf("ErrorClass = %q", res.ErrorClass)
	}
	if string(res.ScreenshotPNG) != string(png) {
		t.Fatalf("screenshot mismatch: %q", res.ScreenshotPNG)
	}
}

func TestLoginDefaultsErrorClassAndMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAgentJSON(w, http.StatusInternalServerError, agentResponse{OK: false})
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "", 5*time.Second).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.ErrorClass != ErrUnknown {
		t.Fatalf("ErrorClass = %q, want %q", res.ErrorClass, ErrUnknown)
	}
	if res.Error == "" {
		t.Fatal("expected a fallback error message")
	}
}

func TestLoginUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "wrong", 5*time.Second).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.ErrorClass != ErrAgentUnauthorized {
		t.Fatalf("ErrorClass = %q", res.ErrorClass)
	}
}

func TestLoginNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "", 5*time.Second).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.ErrorClass != ErrAgentBadStatus {
		t.Fatalf("ErrorClass = %q", res.ErrorClass)
	}
}

func TestLoginUnreachableAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	res, err := NewClient(url, "", 2*time.Second).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.OK {
		t.Fatal("expected failure")
	}
	if res.ErrorClass != ErrAgentUnreachable {
		t.Fatalf("ErrorClass = %q, want %q", res.ErrorClass, ErrAgentUnreachable)
	}
}

func TestLoginTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		writeAgentJSON(w, http.StatusOK, agentResponse{OK: true})
	}))
	defer srv.Close()
	defer close(release)

	res, err := NewClient(srv.URL, "", 100*time.Millisecond).Login(context.Background(), LoginRequest{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.ErrorClass != ErrTimeout {
		t.Fatalf("ErrorClass = %q, want %q", res.ErrorClass, ErrTimeout)
	}
}

func TestHealth(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok", 5*time.Second)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}

	status = http.StatusServiceUnavailable
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error for non-200 health")
	}

	var nilClient *Client
	if err := nilClient.Health(context.Background()); err == nil {
		t.Fatal("expected error from nil client Health")
	}
}

func writeAgentJSON(w http.ResponseWriter, status int, body agentResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
