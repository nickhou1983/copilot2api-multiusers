package copilot

import (
	"net/http"
	"testing"
)

func newReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest("POST", "https://api.githubcopilot.com/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestAddHeadersEditorProfile(t *testing.T) {
	req := newReq(t)
	AddHeaders(req, "tok") // defaults to editor profile

	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Openai-Intent"); got != editorIntent {
		t.Errorf("Openai-Intent = %q, want %q", got, editorIntent)
	}
	if got := req.Header.Get("X-Github-Api-Version"); got != editorAPIVersion {
		t.Errorf("X-Github-Api-Version = %q, want %q", got, editorAPIVersion)
	}
	if got := req.Header.Get("X-Initiator"); got != "" {
		t.Errorf("editor profile should not set X-Initiator, got %q", got)
	}
	if got := req.Header.Get("Copilot-Integration-Id"); got != "vscode-chat" {
		t.Errorf("Copilot-Integration-Id = %q", got)
	}
	if req.Header.Get("X-Request-Id") == "" {
		t.Error("X-Request-Id should be generated")
	}
}

func TestAddHeadersOpenCodeProfile(t *testing.T) {
	req := newReq(t)
	AddHeadersProfile(req, "tok", ProfileOpenCode)

	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Openai-Intent"); got != openCodeIntent {
		t.Errorf("Openai-Intent = %q, want %q", got, openCodeIntent)
	}
	if got := req.Header.Get("X-Github-Api-Version"); got != openCodeAPIVersion {
		t.Errorf("X-Github-Api-Version = %q, want %q", got, openCodeAPIVersion)
	}
	if got := req.Header.Get("X-Initiator"); got != "user" {
		t.Errorf("X-Initiator = %q, want user", got)
	}
}

func TestAddHeadersOpenCodePreservesInitiator(t *testing.T) {
	req := newReq(t)
	req.Header.Set("X-Initiator", "agent")
	AddHeadersProfile(req, "tok", ProfileOpenCode)
	if got := req.Header.Get("X-Initiator"); got != "agent" {
		t.Errorf("X-Initiator = %q, want preserved agent", got)
	}
}
