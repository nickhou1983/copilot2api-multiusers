package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whtsky/copilot2api/internal/copilot"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newDirectClient(t *testing.T, githubToken string) *Client {
	t.Helper()
	dir := t.TempDir()
	if githubToken != "" {
		creds := `{"github_token":"` + githubToken + `"}`
		if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(creds), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewClient(dir, ModeDirect)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestDirectMode_GetTokenReturnsGitHubToken(t *testing.T) {
	c := newDirectClient(t, "gho_direct123")
	tok, err := c.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok != "gho_direct123" {
		t.Errorf("GetToken = %q, want raw GitHub token", tok)
	}
}

func TestDirectMode_GetTokenErrorsWithoutGitHubToken(t *testing.T) {
	c := newDirectClient(t, "")
	if _, err := c.GetToken(context.Background()); err == nil {
		t.Fatal("expected error without stored GitHub token")
	}
}

func TestDirectMode_GetBaseURLIsStatic(t *testing.T) {
	c := newDirectClient(t, "gho_direct123")
	if got := c.GetBaseURL(); got != DirectBaseURL {
		t.Errorf("GetBaseURL = %q, want %q", got, DirectBaseURL)
	}
}

func TestDirectMode_GetValidTokenRejected(t *testing.T) {
	c := newDirectClient(t, "gho_direct123")
	if _, err := c.GetValidToken(context.Background()); err == nil {
		t.Fatal("expected GetValidToken to be rejected in direct mode")
	}
}

func TestHeaderProfileByMode(t *testing.T) {
	direct := newDirectClient(t, "gho_x")
	if got := direct.HeaderProfile(); got != copilot.ProfileOpencode {
		t.Errorf("direct HeaderProfile = %q, want opencode", got)
	}
	exchange, err := NewClient(t.TempDir(), ModeExchange)
	if err != nil {
		t.Fatal(err)
	}
	if got := exchange.HeaderProfile(); got != copilot.ProfileEditor {
		t.Errorf("exchange HeaderProfile = %q, want editor", got)
	}
}

func TestNewClientDefaultsToExchange(t *testing.T) {
	c, err := NewClient(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode() != ModeExchange {
		t.Errorf("Mode = %q, want exchange", c.Mode())
	}
}

func TestDirectMode_GetUsageInfo(t *testing.T) {
	oldHTTPClient := sharedHTTPClient
	t.Cleanup(func() {
		sharedHTTPClient = oldHTTPClient
	})

	sharedHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != CopilotUserURL {
				t.Errorf("URL = %q, want %q", req.URL.String(), CopilotUserURL)
			}
			if got := req.Header.Get("Authorization"); got != "token gho_direct123" {
				t.Errorf("Authorization = %q, want direct OAuth token", got)
			}
			if got := req.Header.Get("Editor-Version"); got != copilot.EditorVersion {
				t.Errorf("Editor-Version = %q, want %q", got, copilot.EditorVersion)
			}
			if got := req.Header.Get("Editor-Plugin-Version"); got != copilot.EditorPluginVersion {
				t.Errorf("Editor-Plugin-Version = %q, want %q", got, copilot.EditorPluginVersion)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"copilot_plan": "individual",
					"quota_reset_date": "2026-09-01",
					"quota_snapshots": {
						"premium_interactions": {
							"entitlement": 300,
							"remaining": 250,
							"percent_remaining": 83
						}
					}
				}`)),
			}, nil
		}),
	}

	c := newDirectClient(t, "gho_direct123")
	info, err := c.GetUsageInfo(context.Background())
	if err != nil {
		t.Fatalf("GetUsageInfo: %v", err)
	}
	if info.CopilotPlan != "individual" {
		t.Errorf("CopilotPlan = %q, want individual", info.CopilotPlan)
	}
	if info.QuotaResetDate != "2026-09-01" {
		t.Errorf("QuotaResetDate = %q, want 2026-09-01", info.QuotaResetDate)
	}
	if _, ok := info.QuotaSnapshots["premium_interactions"]; !ok {
		t.Errorf("QuotaSnapshots = %v, want premium_interactions", info.QuotaSnapshots)
	}

	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal usage info: %v", err)
	}
	if strings.Contains(string(encoded), `"sku"`) {
		t.Errorf("direct usage response includes exchange-only fields: %s", encoded)
	}
}
