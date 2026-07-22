package copilot

import (
	"net/http"
	"testing"
)

func newReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest("POST", "https://api.example.com/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestAddHeadersForProfile_Editor(t *testing.T) {
	req := newReq(t)
	AddHeadersForProfile(req, "tok", ProfileEditor)

	want := map[string]string{
		"Authorization":          "Bearer tok",
		"User-Agent":             CopilotUserAgent,
		"Editor-Version":         EditorVersion,
		"Editor-Plugin-Version":  EditorPluginVersion,
		"Copilot-Integration-Id": "vscode-chat",
		"Openai-Intent":          "conversation-agent",
		"Content-Type":           "application/json",
		"X-Github-Api-Version":   "2026-06-01",
	}
	for k, v := range want {
		if got := req.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if req.Header.Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id to be generated")
	}
	if req.Header.Get("X-Initiator") != "" {
		t.Error("editor profile must not set X-Initiator")
	}
}

func TestAddHeadersForProfile_Opencode(t *testing.T) {
	req := newReq(t)
	AddHeadersForProfile(req, "tok", ProfileOpencode)

	want := map[string]string{
		"Authorization":        "Bearer tok",
		"User-Agent":           OpencodeUserAgent,
		"Openai-Intent":        "conversation-edits",
		"Content-Type":         "application/json",
		"X-Github-Api-Version": "2026-06-01",
		"X-Initiator":          "user",
	}
	for k, v := range want {
		if got := req.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	for _, k := range []string{"Editor-Version", "Editor-Plugin-Version", "Copilot-Integration-Id", "X-Request-Id"} {
		if got := req.Header.Get(k); got != "" {
			t.Errorf("opencode profile must not set %s (got %q)", k, got)
		}
	}
}

func TestAddHeadersDefaultsToEditor(t *testing.T) {
	req := newReq(t)
	AddHeaders(req, "tok")
	if got := req.Header.Get("User-Agent"); got != CopilotUserAgent {
		t.Errorf("User-Agent = %q, want editor profile default", got)
	}
}
