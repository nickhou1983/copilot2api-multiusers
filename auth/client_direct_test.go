package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/whtsky/copilot2api/internal/copilot"
)

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
