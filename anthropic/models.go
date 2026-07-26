package anthropic

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/whtsky/copilot2api/internal/models"
)

// modelAliases maps model name variants to their Copilot equivalents.
// Only needed for non-obvious mappings that can't be derived algorithmically.
var modelAliases = map[string]string{
	// Non-obvious version mappings (pre-4.5 naming used single-digit versions)
	"claude-opus-4":  "claude-opus-4.5",
	"claude-sonnet-4": "claude-sonnet-4", // identity, but here for documentation
}

// versionHyphenRe matches hyphen-separated version numbers like "4-6" or "4-5"
// that appear after a letter segment (e.g. "opus-4-6"). Both digits must be single
// to avoid matching date components like "04-14" or "20-25" in "2025-04-14".
var versionHyphenRe = regexp.MustCompile(`([a-zA-Z]-)(\d)-(\d)([^0-9]|$)`)

// dateSuffixRe matches an 8-digit date suffix like "-20250514" or "-20251001"
// at the end of a model ID (optionally followed by more digits for timestamps).
var dateSuffixRe = regexp.MustCompile(`-(\d{8,})$`)

// context1mRe matches the "context-1m" token in the anthropic-beta header,
// used by Claude Code to signal the 1M context window variant.
var context1mRe = regexp.MustCompile(`\bcontext-1m\b`)

// oneMillionContextTokens is the threshold (in tokens) at which a model is
// considered to already provide a 1M context window natively.
const oneMillionContextTokens = 1_000_000

// resolve1MContextModel decides the effective model ID when the client requests
// the 1M context window via the "anthropic-beta: context-1m" header.
//
// Copilot historically exposed the 1M variant as a separate model ID with a
// "-1m" suffix (e.g. "claude-sonnet-4-1m"). Newer Claude models advertise a 1M
// context window on the base model ID directly, with no "-1m" variant in the
// model list. Blindly appending "-1m" would fabricate a non-existent model ID
// for those, breaking capability detection and routing.
//
// Resolution order:
//  1. Already has a "-1m" suffix -> use as-is.
//  2. Base model already advertises >= 1M context -> use the base ID.
//  3. A "<model>-1m" variant exists upstream -> use that variant.
//  4. Otherwise -> leave the base model unchanged.
func resolve1MContextModel(modelID string, infoMap map[string]*models.Info) string {
	if strings.HasSuffix(modelID, "-1m") {
		return modelID
	}
	if models.MaxContextWindow(infoMap[modelID]) >= oneMillionContextTokens {
		return modelID
	}
	if variant := modelID + "-1m"; infoMap[variant] != nil {
		return variant
	}
	return modelID
}

// resolveModelAlias returns the canonical model ID for Copilot's model list.
// It applies the following transformations in order:
//  1. Strip date suffixes (e.g. "-20250514")
//  2. Normalize hyphen-separated versions to dot-separated (e.g. "4-6" → "4.6")
//  3. Check explicit alias overrides for non-obvious mappings
func resolveModelAlias(modelID string) string {
	// Step 1: Strip date suffix (e.g. "claude-opus-4-6-20250514" → "claude-opus-4-6")
	stripped := dateSuffixRe.ReplaceAllString(modelID, "")
	if stripped == "" {
		stripped = modelID // safety: don't strip everything
	}

	// Step 2: Normalize hyphen-separated versions to dot-separated
	normalized := versionHyphenRe.ReplaceAllString(stripped, "${1}${2}.${3}${4}")

	// Step 3: Check explicit aliases (e.g. "claude-opus-4" → "claude-opus-4.5")
	if alias, ok := modelAliases[normalized]; ok {
		return alias
	}

	// If normalization changed anything, return the normalized form
	if normalized != modelID {
		return normalized
	}

	return modelID
}

// getModelInfo returns cached model info, fetching from upstream if needed.
func (h *Handler) getModelInfo(ctx context.Context, modelID string) (*models.Info, bool) {
	modelID = resolveModelAlias(modelID)

	infoMap, err := h.models.GetInfo(ctx)
	if err != nil {
		slog.Error("failed to fetch models for capability detection", "error", err)
		return nil, true
	}

	return infoMap[modelID], false
}

func modelSupportsEndpoint(info *models.Info, endpoint string) bool {
	return models.SupportsEndpoint(info, endpoint)
}
