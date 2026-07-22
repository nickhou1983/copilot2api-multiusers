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
	// OpencodeVersion is the version advertised by the opencode header profile.
	OpencodeVersion   = "0.4.2"
	OpencodeUserAgent = "opencode/" + OpencodeVersion
)

// Profile selects the set of outbound headers sent to the Copilot API.
type Profile string

const (
	// ProfileEditor mimics the VS Code Copilot Chat extension (exchange mode).
	ProfileEditor Profile = "editor"
	// ProfileOpencode mimics the opencode CLI (direct mode).
	ProfileOpencode Profile = "opencode"
)

// AddHeaders adds the default (editor profile) Copilot headers to the request.
func AddHeaders(req *http.Request, token string) {
	AddHeadersForProfile(req, token, ProfileEditor)
}

// AddHeadersForProfile adds the Copilot headers for the given profile.
func AddHeadersForProfile(req *http.Request, token string, profile Profile) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	if profile == ProfileOpencode {
		req.Header.Set("User-Agent", OpencodeUserAgent)
		req.Header.Set("Openai-Intent", "conversation-edits")
		req.Header.Set("X-Github-Api-Version", "2026-06-01")
		req.Header.Set("X-Initiator", "user")
		return
	}

	req.Header.Set("User-Agent", CopilotUserAgent)
	req.Header.Set("Editor-Version", EditorVersion)
	req.Header.Set("Editor-Plugin-Version", EditorPluginVersion)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Openai-Intent", "conversation-agent")
	req.Header.Set("X-Github-Api-Version", "2026-06-01")

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
