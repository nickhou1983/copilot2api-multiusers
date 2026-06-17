package stats

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUsageOpenAI(t *testing.T) {
	body := []byte(`{"model":"gpt-5","usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":30}}}`)
	model, u, ok := ParseUsage("/chat/completions", body)
	if !ok {
		t.Fatal("expected ok")
	}
	if model != "gpt-5" {
		t.Fatalf("model = %q", model)
	}
	// Input excludes cached: 100 - 30 = 70.
	if u.Input != 70 || u.Output != 20 || u.Cached != 30 || u.CacheCreation != 0 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestParseUsageResponses(t *testing.T) {
	body := []byte(`{"model":"gpt-5","status":"completed","usage":{"input_tokens":80,"output_tokens":12,"input_tokens_details":{"cached_tokens":50}}}`)
	model, u, ok := ParseUsage("/responses", body)
	if !ok {
		t.Fatal("expected ok")
	}
	if model != "gpt-5" || u.Input != 30 || u.Output != 12 || u.Cached != 50 {
		t.Fatalf("model=%q usage=%+v", model, u)
	}
}

func TestParseUsageAnthropic(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4.6","usage":{"input_tokens":40,"output_tokens":9,"cache_read_input_tokens":15,"cache_creation_input_tokens":7}}`)
	model, u, ok := ParseUsage("/v1/messages", body)
	if !ok {
		t.Fatal("expected ok")
	}
	if model != "claude-opus-4.6" {
		t.Fatalf("model = %q", model)
	}
	// Anthropic input_tokens already excludes cache read.
	if u.Input != 40 || u.Output != 9 || u.Cached != 15 || u.CacheCreation != 7 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestParseUsageUnknownEndpoint(t *testing.T) {
	if _, _, ok := ParseUsage("/models", []byte(`{}`)); ok {
		t.Fatal("expected ok=false for /models")
	}
}

func TestParseUsageClampsNegative(t *testing.T) {
	// cached_tokens > prompt_tokens should clamp input to 0, not go negative.
	body := []byte(`{"model":"m","usage":{"prompt_tokens":10,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":25}}}`)
	_, u, ok := ParseUsage("/chat/completions", body)
	if !ok || u.Input != 0 || u.Cached != 25 {
		t.Fatalf("ok=%v usage=%+v", ok, u)
	}
}

// recordingStore captures records for scanner tests.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "stats.json"))
}

func drain(rc io.ReadCloser) {
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}

func TestUsageScannerOpenAIStream(t *testing.T) {
	store := newTestStore(t)
	stream := "data: {\"model\":\"gpt-5\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"model\":\"gpt-5\",\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"prompt_tokens_details\":{\"cached_tokens\":4}}}\n\n" +
		"data: [DONE]\n\n"
	rc := NewUsageScanner(store.Recorder("a"), "/chat/completions", io.NopCloser(strings.NewReader(stream)))
	drain(rc)

	snap := store.Snapshot()
	if len(snap) != 1 || len(snap[0].Models) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	m := snap[0].Models[0]
	if m.Model != "gpt-5" || m.Requests != 1 || m.Input != 8 || m.Output != 3 || m.Cached != 4 {
		t.Fatalf("model stats = %+v", m)
	}
}

func TestUsageScannerOpenAIStreamNoUsage(t *testing.T) {
	// Without stream_options.include_usage there is no usage block: request is
	// still counted but tokens stay zero.
	store := newTestStore(t)
	stream := "data: {\"model\":\"gpt-5\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: [DONE]\n\n"
	rc := NewUsageScanner(store.Recorder("a"), "/chat/completions", io.NopCloser(strings.NewReader(stream)))
	drain(rc)

	snap := store.Snapshot()
	if len(snap) != 1 || snap[0].Totals.Requests != 1 || snap[0].Totals.Input != 0 {
		t.Fatalf("expected 1 request 0 tokens: %+v", snap)
	}
	if snap[0].Models[0].Model != "gpt-5" {
		t.Fatalf("model = %q", snap[0].Models[0].Model)
	}
}

func TestUsageScannerAnthropicStream(t *testing.T) {
	store := newTestStore(t)
	// message_start carries input/cache; message_delta carries output.
	stream := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-opus-4.6\",\"usage\":{\"input_tokens\":40,\"cache_read_input_tokens\":15,\"cache_creation_input_tokens\":7}}}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":9}}\n\n"
	rc := NewUsageScanner(store.Recorder("a"), "/v1/messages", io.NopCloser(strings.NewReader(stream)))
	drain(rc)

	m := store.Snapshot()[0].Models[0]
	if m.Model != "claude-opus-4.6" || m.Input != 40 || m.Output != 9 || m.Cached != 15 || m.CacheCreation != 7 {
		t.Fatalf("model stats = %+v", m)
	}
}

func TestUsageScannerResponsesStream(t *testing.T) {
	store := newTestStore(t)
	stream := "event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5\",\"usage\":{\"input_tokens\":80,\"output_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":50}}}}\n\n"
	rc := NewUsageScanner(store.Recorder("a"), "/responses", io.NopCloser(strings.NewReader(stream)))
	drain(rc)

	m := store.Snapshot()[0].Models[0]
	if m.Model != "gpt-5" || m.Input != 30 || m.Output != 12 || m.Cached != 50 {
		t.Fatalf("model stats = %+v", m)
	}
}

func TestUsageScannerNilRecorderPassthrough(t *testing.T) {
	// Nil recorder => unwrapped body, no panic.
	rc := NewUsageScanner(nil, "/chat/completions", io.NopCloser(strings.NewReader("data: [DONE]\n\n")))
	out, _ := io.ReadAll(rc)
	if string(out) != "data: [DONE]\n\n" {
		t.Fatalf("passthrough mismatch: %q", out)
	}
	_ = rc.Close()
}

func TestStoreLoadFlushRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	s := NewStore(path)
	s.Recorder("alice").Record("gpt-5", Usage{Input: 100, Output: 20, Cached: 30})
	if err := s.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := s2.Snapshot()
	if len(snap) != 1 || snap[0].ID != "alice" {
		t.Fatalf("loaded = %+v", snap)
	}
	if snap[0].Totals.Input != 100 || snap[0].Totals.Cached != 30 {
		t.Fatalf("loaded totals = %+v", snap[0].Totals)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(s.Snapshot()) != 0 {
		t.Fatal("expected empty snapshot")
	}
}

func TestStoreReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	s := NewStore(path)
	s.Recorder("alice").Record("m", Usage{Input: 5})
	s.Recorder("bob").Record("m", Usage{Input: 9})
	if err := s.Reset("alice"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].ID != "bob" {
		t.Fatalf("after reset = %+v", snap)
	}
	// Reset persisted to disk.
	s2 := NewStore(path)
	_ = s2.Load()
	if len(s2.Snapshot()) != 1 {
		t.Fatalf("reset not persisted: %+v", s2.Snapshot())
	}
}

func TestRecorderEmptyAccountDefaults(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "stats.json"))
	s.Recorder("").Record("m", Usage{Input: 1})
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].ID != "default" {
		t.Fatalf("empty account should map to default: %+v", snap)
	}
}

func TestRecordEmptyModelBucketsUnknown(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "stats.json"))
	s.Recorder("a").Record("", Usage{})
	snap := s.Snapshot()
	if snap[0].Models[0].Model != "unknown" || snap[0].Models[0].Requests != 1 {
		t.Fatalf("expected unknown bucket with 1 request: %+v", snap[0].Models)
	}
}

func TestFlushNotDirtyNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	s := NewStore(path)
	if err := s.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Nothing recorded => not dirty => no file written.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file, stat err = %v", err)
	}
}
