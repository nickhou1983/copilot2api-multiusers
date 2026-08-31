// Package loginagent talks to the dedicated browser-automation container that
// completes a GitHub Device Flow authorization on behalf of an account.
//
// The agent holds plaintext GitHub credentials during a login, so it must only
// ever be reachable on a private container network and should be protected with
// a shared token.
package loginagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variables that configure the agent connection.
const (
	EnvURL     = "COPILOT2API_LOGIN_AGENT_URL"
	EnvToken   = "COPILOT2API_LOGIN_AGENT_TOKEN"
	EnvTimeout = "COPILOT2API_LOGIN_AGENT_TIMEOUT_SECONDS"

	// TokenHeader carries the shared secret to the agent.
	TokenHeader = "X-Agent-Token"

	// DefaultTimeout bounds a single automated login attempt. Browser startup
	// plus GitHub login plus device authorization comfortably fits well under
	// this, but a wedged page should not block forever.
	DefaultTimeout = 180 * time.Second
)

// Failure classes reported by the agent. They let the admin UI explain what
// went wrong and whether a manual fallback is required.
const (
	ErrInvalidCredentials = "invalid_credentials"
	ErrTwoFactorRequired  = "two_factor_required"
	ErrDeviceVerification = "device_verification"
	ErrCaptcha            = "captcha"
	ErrCodeRejected       = "code_rejected"
	ErrTimeout            = "timeout"
	ErrUnknown            = "unknown"
	ErrAgentUnreachable   = "agent_unreachable"
	ErrAgentBadStatus     = "agent_bad_status"
	ErrAgentBadResponse   = "agent_bad_response"
	ErrAgentUnauthorized  = "agent_unauthorized"
)

// LoginRequest asks the agent to authorize one device code.
type LoginRequest struct {
	AccountID       string `json:"account_id"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
}

// LoginResult reports the outcome of an automated login attempt.
type LoginResult struct {
	OK bool `json:"ok"`
	// Step names the automation stage reached, e.g. "login", "device_code",
	// "authorize", "done". Surfaced in the admin UI for troubleshooting.
	Step string `json:"step"`
	// ErrorClass is one of the Err* constants above when OK is false.
	ErrorClass string `json:"error_class"`
	// Error is a human-readable failure message.
	Error string `json:"error"`
	// ScreenshotPNG holds a failure screenshot, decoded from the agent's
	// base64 payload. May be empty.
	ScreenshotPNG []byte `json:"-"`
}

// agentResponse mirrors the JSON the agent returns.
type agentResponse struct {
	OK         bool   `json:"ok"`
	Step       string `json:"step"`
	ErrorClass string `json:"error_class"`
	Error      string `json:"error"`
	Screenshot string `json:"screenshot"` // base64-encoded PNG
}

// Client is an HTTP client for the login agent.
type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	http    *http.Client
}

// NewClient builds a client for the given agent base URL. A blank baseURL
// returns nil, which callers treat as "automated login disabled".
func NewClient(baseURL, token string, timeout time.Duration) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		timeout: timeout,
		// Allow a little slack over the per-request context so context
		// cancellation, not the transport, produces the error.
		http: &http.Client{Timeout: timeout + 15*time.Second},
	}
}

// NewClientFromEnv builds a client from COPILOT2API_LOGIN_AGENT_* environment
// variables. It returns nil when no agent URL is configured, which disables
// automated login without affecting the existing manual device flow.
func NewClientFromEnv() *Client {
	timeout := DefaultTimeout
	if v := os.Getenv(EnvTimeout); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	return NewClient(os.Getenv(EnvURL), os.Getenv(EnvToken), timeout)
}

// BaseURL returns the configured agent URL (never includes the shared token).
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// Timeout returns the per-login timeout.
func (c *Client) Timeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.timeout
}

// Login asks the agent to complete the device authorization. A non-nil error is
// returned only when the client is unusable; transport and automation failures
// come back as a LoginResult with OK false so the caller can surface them.
func (c *Client) Login(ctx context.Context, req LoginRequest) (*LoginResult, error) {
	if c == nil {
		return nil, fmt.Errorf("login agent is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set(TokenHeader, c.token)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		class := ErrAgentUnreachable
		if ctx.Err() != nil {
			class = ErrTimeout
		}
		return &LoginResult{ErrorClass: class, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return &LoginResult{ErrorClass: ErrAgentBadResponse, Error: err.Error()}, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return &LoginResult{ErrorClass: ErrAgentUnauthorized, Error: "login agent rejected the shared token"}, nil
	}

	var ar agentResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		if resp.StatusCode != http.StatusOK {
			return &LoginResult{
				ErrorClass: ErrAgentBadStatus,
				Error:      fmt.Sprintf("login agent returned status %d", resp.StatusCode),
			}, nil
		}
		return &LoginResult{ErrorClass: ErrAgentBadResponse, Error: "failed to parse login agent response"}, nil
	}

	result := &LoginResult{
		OK:         ar.OK,
		Step:       ar.Step,
		ErrorClass: ar.ErrorClass,
		Error:      ar.Error,
	}
	if !result.OK && result.ErrorClass == "" {
		result.ErrorClass = ErrUnknown
	}
	if !result.OK && result.Error == "" {
		result.Error = fmt.Sprintf("login agent returned status %d", resp.StatusCode)
	}
	if ar.Screenshot != "" {
		if png, decErr := base64.StdEncoding.DecodeString(ar.Screenshot); decErr == nil {
			result.ScreenshotPNG = png
		}
	}
	return result, nil
}

// Health probes the agent's readiness endpoint.
func (c *Client) Health(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("login agent is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set(TokenHeader, c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login agent health returned status %d", resp.StatusCode)
	}
	return nil
}
