package copilot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Exported constants for User-Agent and version headers.
const (
	CopilotUserAgent    = "GitHubCopilotChat/0.39.0"
	EditorVersion       = "vscode/1.111.0"
	EditorPluginVersion = "copilot-chat/0.39.0"
)

// Header profiles select which outbound header set is applied to Copilot
// requests. ProfileEditor mimics the VSCode Copilot Chat client (used by the
// two-step exchange flow); ProfileOpenCode mirrors OpenCode's direct-token
// flow against api.githubcopilot.com.
const (
	ProfileEditor   = "editor"
	ProfileOpenCode = "opencode"
)

// Openai-Intent / X-Github-Api-Version values per profile.
const (
	editorIntent     = "conversation-agent"
	editorAPIVersion = "2025-04-01"

	openCodeIntent     = "conversation-edits"
	openCodeAPIVersion = "2026-06-01"
)

// AddHeaders adds required Copilot headers to the request using the default
// editor profile.
func AddHeaders(req *http.Request, token string) {
	AddHeadersProfile(req, token, ProfileEditor)
}

// AddHeadersProfile adds required Copilot headers to the request using the
// given header profile (ProfileEditor or ProfileOpenCode).
func AddHeadersProfile(req *http.Request, token, profile string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", CopilotUserAgent)
	req.Header.Set("Editor-Version", EditorVersion)
	req.Header.Set("Editor-Plugin-Version", EditorPluginVersion)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Content-Type", "application/json")

	switch profile {
	case ProfileOpenCode:
		req.Header.Set("Openai-Intent", openCodeIntent)
		req.Header.Set("X-Github-Api-Version", openCodeAPIVersion)
		// x-initiator distinguishes user- vs agent-initiated turns. Full
		// per-request detection is out of scope; default to "user".
		if req.Header.Get("X-Initiator") == "" {
			req.Header.Set("X-Initiator", "user")
		}
	default:
		req.Header.Set("Openai-Intent", editorIntent)
		req.Header.Set("X-Github-Api-Version", editorAPIVersion)
	}

	// Generate request ID if not present
	if req.Header.Get("X-Request-Id") == "" {
		req.Header.Set("X-Request-Id", GenerateRequestID())
	}
}

// GenerateRequestID generates a unique request ID using crypto/rand
func GenerateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		slog.Error("crypto/rand.Read failed", "error", err)
		return fmt.Sprintf("req_fallback_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(b)
}
