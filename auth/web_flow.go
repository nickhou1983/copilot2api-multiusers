package auth

import (
	"fmt"
	"log/slog"
	"time"
)

// IsAuthenticated reports whether a GitHub token is stored for this account.
// A stored GitHub token is sufficient to (re)mint Copilot tokens on demand.
func (c *Client) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.creds.GitHubToken != ""
}

// StoredTokenInfo is a read-only snapshot of the credentials persisted for an
// account. Returned by StoredTokens for display in the admin UI.
type StoredTokenInfo struct {
	GitHubToken      string    `json:"github_token"`
	CopilotToken     string    `json:"copilot_token"`
	CopilotExpiresAt time.Time `json:"copilot_expires_at"`
	CopilotUsable    bool      `json:"copilot_usable"`
	BaseURL          string    `json:"base_url"`
}

// StoredTokens returns the tokens currently saved for this account.
func (c *Client) StoredTokens() StoredTokenInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	info := StoredTokenInfo{GitHubToken: c.creds.GitHubToken}
	if c.creds.CopilotToken != nil {
		info.CopilotToken = c.creds.CopilotToken.Token
		info.CopilotExpiresAt = c.creds.CopilotToken.ExpiresAt
		info.CopilotUsable = c.creds.CopilotToken.IsTokenUsable()
		info.BaseURL = c.creds.CopilotToken.BaseURL
	}
	return info
}

// StartDeviceFlow initiates a GitHub Device Flow and returns the device/user
// codes so a caller (e.g. a web UI) can present them to the user. Use
// CompleteDeviceFlow with the returned DeviceCode to finish authentication.
func (c *Client) StartDeviceFlow() (*DeviceCodeResponse, error) {
	resp, err := InitiateDeviceFlow(c.enterpriseDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate device flow: %w", err)
	}
	return resp, nil
}

// CompleteDeviceFlow polls GitHub until the user authorizes the device code (or
// the timeout elapses), then stores the GitHub token and mints a Copilot token.
// It is the non-interactive counterpart to performDeviceFlow, suitable for
// web-driven authentication. Serialized via refreshMu so it does not race with
// token refreshes.
func (c *Client) CompleteDeviceFlow(deviceCode string, interval int, timeout time.Duration) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	accessToken, err := PollForAccessToken(c.enterpriseDomain, deviceCode, interval, timeout)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	c.mu.Lock()
	c.creds.GitHubToken = accessToken
	c.creds.AuthMode = c.mode
	c.creds.EnterpriseURL = c.enterpriseDomain
	c.mu.Unlock()

	if c.mode == ModeDirect {
		c.saveCreds()
		slog.Info("device flow authentication completed", "mode", c.mode)
		return nil
	}

	if err := c.refreshCopilotToken(); err != nil {
		return fmt.Errorf("failed to get copilot token: %w", err)
	}

	slog.Info("device flow authentication completed", "mode", c.mode)
	return nil
}
