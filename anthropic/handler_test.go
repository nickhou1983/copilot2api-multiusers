package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestReadSSEEventMultiLineData(t *testing.T) {
	input := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",",
		"data: \"delta\":\"hello\"}",
		"",
	}, "\n")

	reader := bufio.NewReader(strings.NewReader(input))
	event, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("readSSEEvent returned error: %v", err)
	}
	if event == nil {
		t.Fatal("readSSEEvent returned nil event")
	}

	if event.Event != "response.output_text.delta" {
		t.Fatalf("event type = %q, want %q", event.Event, "response.output_text.delta")
	}

	wantData := "{\"type\":\"response.output_text.delta\",\n\"delta\":\"hello\"}"
	if event.Data != wantData {
		t.Fatalf("event data = %q, want %q", event.Data, wantData)
	}
}

func TestReadSSEEventEOFWithoutData(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(""))
	event, err := readSSEEvent(reader)
	if err == nil {
		t.Fatal("expected EOF error")
	}
	if err != io.EOF {
		t.Fatalf("error = %v, want io.EOF", err)
	}
	if event != nil {
		t.Fatalf("event = %#v, want nil", event)
	}
}

func TestNormalizeNativeMessagesBody_RemovesCacheControlScope(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-6-20250514",
		"context_management": {"type": "auto"},
		"system": [
			{"type": "text", "text": "one"},
			{"type": "text", "text": "two", "cache_control": {"type": "ephemeral", "ttl": "1h", "scope": "workspace"}}
		],
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hi", "cache_control": {"type": "ephemeral", "scope": "tool"}}]}
		],
		"max_tokens": 16
	}`)

	normalized, err := normalizeNativeMessagesBody(body, "claude-opus-4.6", true)
	if err != nil {
		t.Fatalf("normalizeNativeMessagesBody returned error: %v", err)
	}

	info := inspectCacheControl(normalized)
	if info.ScopeCount != 0 {
		t.Fatalf("ScopeCount = %d, want 0; paths=%v", info.ScopeCount, info.ScopePaths)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		t.Fatalf("failed to decode normalized body: %v", err)
	}

	if decoded["model"] != "claude-opus-4.6" {
		t.Fatalf("model = %v, want claude-opus-4.6", decoded["model"])
	}
	if cm, ok := decoded["context_management"].(map[string]interface{}); !ok {
		t.Fatalf("context_management was dropped, want it preserved")
	} else if cm["type"] != "auto" {
		t.Fatalf("context_management.type = %v, want auto", cm["type"])
	}

	system := decoded["system"].([]interface{})
	cacheControl := system[1].(map[string]interface{})["cache_control"].(map[string]interface{})
	if cacheControl["type"] != "ephemeral" {
		t.Fatalf("system cache_control.type = %v, want ephemeral", cacheControl["type"])
	}
	if cacheControl["ttl"] != "1h" {
		t.Fatalf("system cache_control.ttl = %v, want 1h", cacheControl["ttl"])
	}
	if _, ok := cacheControl["scope"]; ok {
		t.Fatalf("system cache_control.scope still present")
	}

	messages := decoded["messages"].([]interface{})
	parts := messages[0].(map[string]interface{})["content"].([]interface{})
	messageCacheControl := parts[0].(map[string]interface{})["cache_control"].(map[string]interface{})
	if _, ok := messageCacheControl["scope"]; ok {
		t.Fatalf("message cache_control.scope still present")
	}
}

func TestUpstreamNativeHeaders(t *testing.T) {
	h := upstreamNativeHeaders(false, nil)
	if got := h["anthropic-version"]; got != anthropicVersion {
		t.Fatalf("anthropic-version = %q, want %q", got, anthropicVersion)
	}
	if got := h["anthropic-beta"]; got != interleavedThinkingBeta {
		t.Fatalf("anthropic-beta = %q, want %q (base single token)", got, interleavedThinkingBeta)
	}
	if len(h) != 2 {
		t.Fatalf("headers = %v, want exactly anthropic-version and anthropic-beta", h)
	}
}

func TestUpstreamNativeHeaders_ContextManagement(t *testing.T) {
	h := upstreamNativeHeaders(true, nil)
	want := interleavedThinkingBeta + "," + contextManagementBeta + "," + compactionBeta
	if got := h["anthropic-beta"]; got != want {
		t.Fatalf("anthropic-beta = %q, want %q", got, want)
	}
	if got := h["anthropic-version"]; got != anthropicVersion {
		t.Fatalf("anthropic-version = %q, want %q", got, anthropicVersion)
	}
	if len(h) != 2 {
		t.Fatalf("headers = %v, want exactly anthropic-version and anthropic-beta", h)
	}
}

func TestUpstreamNativeHeaders_ForwardsComputerUseBetasOnly(t *testing.T) {
	h := upstreamNativeHeaders(false, []string{
		" fast-mode-2026-02-01, x-computer-use-2025-11-24, computer-use-2025-11-24 ",
		" Computer-use-2025-01-24, output-300k-2026-03-24 ",
	})
	want := interleavedThinkingBeta + ",computer-use-2025-11-24"
	if got := h["anthropic-beta"]; got != want {
		t.Fatalf("anthropic-beta = %q, want %q", got, want)
	}
}

func TestUpstreamNativeHeaders_DeduplicatesComputerUseBetas(t *testing.T) {
	h := upstreamNativeHeaders(true, []string{
		"computer-use-2025-11-24,computer-use-2025-01-24",
		"computer-use-2025-11-24",
	})
	want := strings.Join([]string{
		interleavedThinkingBeta,
		contextManagementBeta,
		compactionBeta,
		"computer-use-2025-11-24",
		"computer-use-2025-01-24",
	}, ",")
	if got := h["anthropic-beta"]; got != want {
		t.Fatalf("anthropic-beta = %q, want %q", got, want)
	}
}

func TestInspectTopLevelFields_CompactionEdit(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"compact edit", `{"context_management":{"edits":[{"type":"compact_20260112"}]}}`, true},
		{"clear_tool_uses only", `{"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}}`, false},
		{"mixed edits", `{"context_management":{"edits":[{"type":"clear_tool_uses_20250919"},{"type":"compact_20260112"}]}}`, true},
		{"no context_management", `{"model":"m"}`, false},
		{"malformed edits", `{"context_management":{"edits":"nope"}}`, false},
	}
	for _, tc := range cases {
		info := inspectTopLevelFields([]byte(tc.body))
		if info.HasCompactionEdit != tc.want {
			t.Errorf("%s: HasCompactionEdit = %v, want %v", tc.name, info.HasCompactionEdit, tc.want)
		}
	}
}

func TestNormalizeNativeMessagesBody_PreservesContextManagement(t *testing.T) {
	body := []byte(`{"model":"m","context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]},"messages":[],"max_tokens":8}`)

	normalized, err := normalizeNativeMessagesBody(body, "m", false)
	if err != nil {
		t.Fatalf("normalizeNativeMessagesBody returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		t.Fatalf("failed to decode normalized body: %v", err)
	}
	cm, ok := decoded["context_management"].(map[string]interface{})
	if !ok {
		t.Fatal("context_management was dropped, want it preserved")
	}
	if _, ok := cm["edits"]; !ok {
		t.Fatal("context_management.edits missing")
	}
}
