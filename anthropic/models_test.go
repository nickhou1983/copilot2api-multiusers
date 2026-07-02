package anthropic

import (
	"testing"

	"github.com/whtsky/copilot2api/internal/models"
)

func TestModelSupportsEndpoint_NormalizedV1Prefix(t *testing.T) {
	info := &models.Info{SupportedEndpoints: []string{"/messages", "/responses"}}

	if !modelSupportsEndpoint(info, "/v1/messages") {
		t.Fatal("expected /v1/messages to match /messages")
	}

	if !modelSupportsEndpoint(info, "/responses") {
		t.Fatal("expected /responses to be supported")
	}

	if modelSupportsEndpoint(info, "/v1/chat/completions") {
		t.Fatal("did not expect /v1/chat/completions to be supported")
	}
}

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Hyphen-separated versions are normalized to dots
		{"claude-opus-4-6", "claude-opus-4.6"},
		{"claude-opus-4-6-fast", "claude-opus-4.6-fast"},
		{"claude-sonnet-4-6", "claude-sonnet-4.6"},
		{"claude-haiku-4-5", "claude-haiku-4.5"},
		{"claude-opus-4-5", "claude-opus-4.5"},
		{"claude-sonnet-4-5", "claude-sonnet-4.5"},

		// Date suffixes are stripped, then version normalization applied
		{"claude-haiku-4-5-20251001", "claude-haiku-4.5"},
		{"claude-haiku-4.5-20251001", "claude-haiku-4.5"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"claude-opus-4-6-20250514", "claude-opus-4.6"},
		{"claude-opus-4.6-20250514", "claude-opus-4.6"},
		{"claude-sonnet-4-6-20250514", "claude-sonnet-4.6"},
		{"claude-sonnet-4.6-20250514", "claude-sonnet-4.6"},
		{"claude-sonnet-4-5-20250514", "claude-sonnet-4.5"},
		{"claude-opus-4-5-20250514", "claude-opus-4.5"},
		{"claude-opus-4.5-20250514", "claude-opus-4.5"},

		// Non-obvious mapping via explicit alias
		{"claude-opus-4-20250514", "claude-opus-4.5"},

		// Future models: should work automatically without new explicit aliases
		{"claude-opus-4-7-20260101", "claude-opus-4.7"},
		{"claude-sonnet-5-0-20260601", "claude-sonnet-5.0"},

		// Generic normalizer: unknown model with hyphen version
		{"claude-sonnet-4-6-fast", "claude-sonnet-4.6-fast"},

		// Already canonical — no change
		{"claude-opus-4.6", "claude-opus-4.6"},
		{"claude-sonnet-4", "claude-sonnet-4"},

		// No version numbers to normalize
		{"claude-sonnet", "claude-sonnet"},

		// Hyphenated dates must NOT be corrupted
		{"claude-sonnet-4-2025-04-14", "claude-sonnet-4-2025-04-14"},
		{"claude-3-5-sonnet-2025-04-14", "claude-3.5-sonnet-2025-04-14"},

		// Non-Claude models pass through unchanged
		{"gpt-5.3-codex", "gpt-5.3-codex"},
		{"gpt-5.4", "gpt-5.4"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveModelAlias(tt.input)
			if got != tt.want {
				t.Errorf("resolveModelAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolve1MContextModel(t *testing.T) {
	// Current Copilot shape: base Claude IDs already advertise a 1M context
	// window, with no "-1m" variants in the list.
	nativeMap := map[string]*models.Info{
		"claude-opus-4.6": {
			ID:           "claude-opus-4.6",
			Capabilities: models.Capability{Limits: models.Limits{MaxContextWindowTokens: 1_000_000}},
		},
		"claude-haiku-4.5": {
			ID:           "claude-haiku-4.5",
			Capabilities: models.Capability{Limits: models.Limits{MaxContextWindowTokens: 200_000}},
		},
	}

	// Legacy Copilot shape: 1M exposed as a separate "-1m" model ID, base reports
	// only 200K.
	legacyMap := map[string]*models.Info{
		"claude-sonnet-4": {
			ID:           "claude-sonnet-4",
			Capabilities: models.Capability{Limits: models.Limits{MaxContextWindowTokens: 200_000}},
		},
		"claude-sonnet-4-1m": {
			ID:           "claude-sonnet-4-1m",
			Capabilities: models.Capability{Limits: models.Limits{MaxContextWindowTokens: 1_000_000}},
		},
	}

	tests := []struct {
		name    string
		modelID string
		infoMap map[string]*models.Info
		want    string
	}{
		// Base already 1M -> no suffix fabricated.
		{"native base already 1M", "claude-opus-4.6", nativeMap, "claude-opus-4.6"},
		// Base not 1M and no variant exists -> leave unchanged (avoid fake ID).
		{"base not 1M, no variant", "claude-haiku-4.5", nativeMap, "claude-haiku-4.5"},
		// Legacy: base not 1M but a "-1m" variant exists -> switch to it.
		{"legacy variant exists", "claude-sonnet-4", legacyMap, "claude-sonnet-4-1m"},
		// Already suffixed -> idempotent.
		{"already 1m suffix", "claude-sonnet-4-1m", legacyMap, "claude-sonnet-4-1m"},
		// Unknown model, empty map -> unchanged.
		{"unknown model", "claude-opus-9.9", map[string]*models.Info{}, "claude-opus-9.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve1MContextModel(tt.modelID, tt.infoMap)
			if got != tt.want {
				t.Errorf("resolve1MContextModel(%q) = %q, want %q", tt.modelID, got, tt.want)
			}
		})
	}
}
