package auth

import (
	"context"
	"testing"

	"github.com/whtsky/copilot2api/internal/copilot"
)

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"github.com":                 "github.com",
		"https://company.ghe.com":    "company.ghe.com",
		"http://company.ghe.com/":    "company.ghe.com",
		"company.ghe.com/":           "company.ghe.com",
		"  https://x.example.com/  ": "x.example.com",
		"":                           "",
	}
	for in, want := range cases {
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDirectBaseURL(t *testing.T) {
	if got := directBaseURL(""); got != CopilotAPIBaseURL {
		t.Errorf("directBaseURL(\"\") = %q, want %q", got, CopilotAPIBaseURL)
	}
	if got := directBaseURL("https://company.ghe.com/"); got != "https://copilot-api.company.ghe.com" {
		t.Errorf("enterprise directBaseURL = %q", got)
	}
}

func TestDirectModeClient(t *testing.T) {
	dir := t.TempDir()
	st, err := NewTokenStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCredentials(&StoredCredentials{GitHubToken: "gho_test"}); err != nil {
		t.Fatal(err)
	}

	c, err := NewClientWithOptions(dir, Options{Mode: ModeDirect})
	if err != nil {
		t.Fatal(err)
	}

	tok, err := c.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok != "gho_test" {
		t.Errorf("direct GetToken = %q, want raw github token", tok)
	}
	if got := c.GetBaseURL(); got != CopilotAPIBaseURL {
		t.Errorf("direct GetBaseURL = %q, want %q", got, CopilotAPIBaseURL)
	}
	if got := c.HeaderProfile(); got != copilot.ProfileOpenCode {
		t.Errorf("direct HeaderProfile = %q, want %q", got, copilot.ProfileOpenCode)
	}
}

func TestDirectModeEnterpriseBaseURL(t *testing.T) {
	dir := t.TempDir()
	c, err := NewClientWithOptions(dir, Options{Mode: ModeDirect, EnterpriseURL: "https://company.ghe.com/"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.GetBaseURL(); got != "https://copilot-api.company.ghe.com" {
		t.Errorf("enterprise GetBaseURL = %q", got)
	}
}

func TestDirectModeRequiresToken(t *testing.T) {
	dir := t.TempDir()
	c, err := NewClientWithOptions(dir, Options{Mode: ModeDirect})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetToken(context.Background()); err == nil {
		t.Fatal("expected error when no github token is stored")
	}
}

func TestExchangeModeDefaults(t *testing.T) {
	dir := t.TempDir()
	c, err := NewClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.HeaderProfile(); got != copilot.ProfileEditor {
		t.Errorf("default HeaderProfile = %q, want %q", got, copilot.ProfileEditor)
	}
	if got := c.GetBaseURL(); got != DefaultBaseURL {
		t.Errorf("default GetBaseURL = %q, want %q", got, DefaultBaseURL)
	}
}

func TestModePersistedInCredentials(t *testing.T) {
	dir := t.TempDir()
	st, err := NewTokenStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Credentials remember mode/enterprise from a prior direct-mode auth.
	if err := st.SaveCredentials(&StoredCredentials{
		GitHubToken:   "gho_test",
		AuthMode:      ModeDirect,
		EnterpriseURL: "company.ghe.com",
	}); err != nil {
		t.Fatal(err)
	}

	// Constructing without explicit options should adopt the persisted values.
	c, err := NewClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.HeaderProfile(); got != copilot.ProfileOpenCode {
		t.Errorf("persisted mode HeaderProfile = %q, want opencode", got)
	}
	if got := c.GetBaseURL(); got != "https://copilot-api.company.ghe.com" {
		t.Errorf("persisted enterprise GetBaseURL = %q", got)
	}
}
