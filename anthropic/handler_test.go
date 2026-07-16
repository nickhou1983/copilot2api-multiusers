package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"slices"
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

func TestExtractComputerUseBetas(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   []string
	}{
		{"empty", nil, nil},
		{"none", []string{"context-1m-2025-08-07,interleaved-thinking-2025-05-14"}, nil},
		{"new only", []string{"computer-use-2025-11-24"}, []string{"computer-use-2025-11-24"}},
		{"old only", []string{"computer-use-2025-01-24"}, []string{"computer-use-2025-01-24"}},
		{"mixed with others", []string{"context-management-2025-06-27,computer-use-2025-11-24"}, []string{"computer-use-2025-11-24"}},
		{"both versions", []string{"computer-use-2025-11-24,computer-use-2025-01-24"}, []string{"computer-use-2025-11-24", "computer-use-2025-01-24"}},
		{"dedup", []string{"computer-use-2025-11-24,computer-use-2025-11-24"}, []string{"computer-use-2025-11-24"}},
		{"spaces around tokens", []string{" context-1m-2025-08-07 , computer-use-2025-11-24 "}, []string{"computer-use-2025-11-24"}},
		{"multiple header lines", []string{"context-1m-2025-08-07", "computer-use-2025-11-24"}, []string{"computer-use-2025-11-24"}},
		{"embedded substring not matched", []string{"xcomputer-use-2025-11-24", "computer-use-2025-11-24-extra"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractComputerUseBetas(tc.values)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("extractComputerUseBetas(%v) = %v, want %v", tc.values, got, tc.want)
			}
		})
	}
}

func TestBuildUpstreamBetaHeaders(t *testing.T) {
	if h := buildUpstreamBetaHeaders(nil); h != nil {
		t.Fatalf("no betas: headers = %v, want nil", h)
	}
	if h := buildUpstreamBetaHeaders([]string{"", ""}); h != nil {
		t.Fatalf("only empty betas: headers = %v, want nil", h)
	}

	h := buildUpstreamBetaHeaders([]string{contextManagementBeta, "computer-use-2025-11-24", contextManagementBeta})
	want := contextManagementBeta + ",computer-use-2025-11-24"
	if got := h["anthropic-beta"]; got != want {
		t.Fatalf("anthropic-beta = %q, want %q (dedup + comma-join)", got, want)
	}
}

func TestCollectUpstreamBetas(t *testing.T) {
	// context_management only
	if got := collectUpstreamBetas(true, false, nil); !slices.Equal(got, []string{contextManagementBeta}) {
		t.Fatalf("context_management only = %v", got)
	}
	// computer-use only (context_management absent)
	if got := collectUpstreamBetas(false, false, []string{"computer-use-2025-11-24"}); !slices.Equal(got, []string{"computer-use-2025-11-24"}) {
		t.Fatalf("computer-use only = %v", got)
	}
	// both, order: context-management first then computer-use
	got := collectUpstreamBetas(true, false, []string{"computer-use-2025-11-24"})
	if !slices.Equal(got, []string{contextManagementBeta, "computer-use-2025-11-24"}) {
		t.Fatalf("both = %v", got)
	}
	// compaction edit adds the compaction beta after context-management
	got = collectUpstreamBetas(true, true, nil)
	if !slices.Equal(got, []string{contextManagementBeta, compactionBeta}) {
		t.Fatalf("compaction = %v", got)
	}
	// none
	if got := collectUpstreamBetas(false, false, nil); len(got) != 0 {
		t.Fatalf("none = %v, want empty", got)
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
