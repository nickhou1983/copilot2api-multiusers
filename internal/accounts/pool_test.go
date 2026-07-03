package accounts

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// poolAccount builds a test account wired to a single handler for all protocols.
func poolAccount(id, key, pool string, h http.Handler) *Account {
	return &Account{ID: id, APIKey: key, Pool: pool, OpenAI: h, Anthropic: h, Gemini: h, Usage: h}
}

// statusHandler writes a fixed status code and body.
func statusHandler(code int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

// drainStatusHandler consumes the request body (to prove replay works) then
// writes a fixed status.
func drainStatusHandler(code int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

// echoHandler echoes the request body with a 200.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})
}

func poolRequest(reg *Registry, p Protocol, key, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(http.MethodPost, "/", nil)
	}
	r.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	reg.Handler(p).ServeHTTP(w, r)
	return w
}

func TestPoolGroupingByKey(t *testing.T) {
	a := poolAccount("a", "k", "team", idHandler("a"))
	b := poolAccount("b", "k", "team", idHandler("b"))
	solo := poolAccount("solo", "k-solo", "", idHandler("solo"))
	reg, err := NewRegistry([]*Account{a, b, solo})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if reg.byKey["k"].Size() != 2 {
		t.Fatalf("pool k size = %d, want 2", reg.byKey["k"].Size())
	}
	if reg.byKey["k-solo"].Size() != 1 {
		t.Fatalf("pool k-solo size = %d, want 1", reg.byKey["k-solo"].Size())
	}
	if len(reg.Accounts()) != 3 {
		t.Fatalf("Accounts = %d, want 3", len(reg.Accounts()))
	}
}

func TestNewRegistrySharedKeyWithoutPoolRejected(t *testing.T) {
	// Same key but no shared pool name -> still a duplicate-key error.
	_, err := NewRegistry([]*Account{
		poolAccount("a", "k", "", idHandler("a")),
		poolAccount("b", "k", "", idHandler("b")),
	})
	if err == nil {
		t.Fatal("expected duplicate key error for shared key without pool")
	}
}

func TestPoolRoundRobin(t *testing.T) {
	a := poolAccount("a", "k", "team", idHandler("a"))
	b := poolAccount("b", "k", "team", idHandler("b"))
	reg, _ := NewRegistry([]*Account{a, b})

	got := make(map[string]int)
	seq := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		w := poolRequest(reg, ProtoOpenAI, "k", "")
		if w.Code != http.StatusOK {
			t.Fatalf("req %d: status %d", i, w.Code)
		}
		got[w.Body.String()]++
		seq = append(seq, w.Body.String())
	}
	if got["a"] != 2 || got["b"] != 2 {
		t.Fatalf("round-robin distribution = %v, want a:2 b:2", got)
	}
	// Consecutive requests must alternate members.
	if seq[0] == seq[1] {
		t.Fatalf("expected alternating members, got %v", seq)
	}
}

func TestPoolFailoverToNextMember(t *testing.T) {
	// Primary rate-limits (429), secondary succeeds. Client should see success.
	a := poolAccount("a", "k", "team", statusHandler(http.StatusTooManyRequests, "rate limited"))
	b := poolAccount("b", "k", "team", statusHandler(http.StatusOK, "ok-b"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (failover)", w.Code)
	}
	if w.Body.String() != "ok-b" {
		t.Fatalf("body = %q, want ok-b", w.Body.String())
	}
}

func TestPoolAllMembersFailReturnsLastError(t *testing.T) {
	a := poolAccount("a", "k", "team", statusHandler(http.StatusTooManyRequests, "a-err"))
	b := poolAccount("b", "k", "team", statusHandler(http.StatusServiceUnavailable, "b-err"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	// order is [a, b]; both retryable; last (b) commits its 503 + body.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (last member error)", w.Code)
	}
	if w.Body.String() != "b-err" {
		t.Fatalf("body = %q, want b-err", w.Body.String())
	}
}

func TestPoolNonRetryableCommitsImmediately(t *testing.T) {
	// Primary returns a non-retryable 400: no failover, error passes through.
	a := poolAccount("a", "k", "team", statusHandler(http.StatusBadRequest, "bad-a"))
	b := poolAccount("b", "k", "team", statusHandler(http.StatusOK, "ok-b"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no failover on non-retryable)", w.Code)
	}
	if w.Body.String() != "bad-a" {
		t.Fatalf("body = %q, want bad-a", w.Body.String())
	}
}

func TestPoolBodyReplayedAcrossMembers(t *testing.T) {
	// Primary consumes the body then 429s; secondary must still see the body.
	a := poolAccount("a", "k", "team", drainStatusHandler(http.StatusTooManyRequests, "err"))
	b := poolAccount("b", "k", "team", echoHandler())
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "hello-world")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "hello-world" {
		t.Fatalf("echoed body = %q, want hello-world (body replay)", w.Body.String())
	}
}

func TestSingleMemberPoolNoFailover(t *testing.T) {
	// A lone account that 429s must NOT be retried (nothing to fail over to);
	// the error passes straight through the fast path.
	a := poolAccount("solo", "k", "", statusHandler(http.StatusTooManyRequests, "rl"))
	reg, _ := NewRegistry([]*Account{a})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 passthrough", w.Code)
	}
	if w.Body.String() != "rl" {
		t.Fatalf("body = %q, want rl", w.Body.String())
	}
}

func TestPoolStreamingSuccessCommits(t *testing.T) {
	streamer := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected captureWriter to implement http.Flusher")
		}
		_, _ = w.Write([]byte("chunk1"))
		if ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("chunk2"))
	})
	a := poolAccount("a", "k", "team", streamer)
	b := poolAccount("b", "k", "team", statusHandler(http.StatusOK, "unused"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "chunk1chunk2" {
		t.Fatalf("streamed body = %q, want chunk1chunk2", w.Body.String())
	}
}

func TestPoolStreamingHeaderFlushCommits(t *testing.T) {
	// A streaming handler sets SSE headers then flushes BEFORE writing any body
	// (as BeginSSE does). The pooled captureWriter must commit a 200 on that
	// flush so headers reach the client immediately.
	streamer := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // pre-stream header flush, no body yet
		}
		_, _ = io.WriteString(w, "data: hi\n\n")
	})
	a := poolAccount("a", "k", "team", streamer)
	b := poolAccount("b", "k", "team", statusHandler(http.StatusOK, "unused"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream (headers committed on flush)", ct)
	}
	if w.Body.String() != "data: hi\n\n" {
		t.Fatalf("body = %q, want SSE line", w.Body.String())
	}
}

func TestPoolStreamingEmptyStreamStillCommitsHeaders(t *testing.T) {
	// Handler sets SSE headers, flushes, then writes nothing (empty stream).
	// The 200 + SSE headers must still reach the client.
	streamer := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	a := poolAccount("a", "k", "team", streamer)
	b := poolAccount("b", "k", "team", statusHandler(http.StatusOK, "unused"))
	reg, _ := NewRegistry([]*Account{a, b})

	w := poolRequest(reg, ProtoOpenAI, "k", "payload")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream on empty stream", ct)
	}
}

func TestConfigValidatePools(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "shared key same pool ok",
			cfg: Config{Accounts: []AccountConfig{
				{ID: "a", APIKey: "k", Pool: "team"},
				{ID: "b", APIKey: "k", Pool: "team"},
			}},
			wantErr: false,
		},
		{
			name: "shared key one without pool",
			cfg: Config{Accounts: []AccountConfig{
				{ID: "a", APIKey: "k", Pool: "team"},
				{ID: "b", APIKey: "k"},
			}},
			wantErr: true,
		},
		{
			name: "shared key different pools",
			cfg: Config{Accounts: []AccountConfig{
				{ID: "a", APIKey: "k", Pool: "team1"},
				{ID: "b", APIKey: "k", Pool: "team2"},
			}},
			wantErr: true,
		},
		{
			name: "pool mapped to two keys",
			cfg: Config{Accounts: []AccountConfig{
				{ID: "a", APIKey: "k1", Pool: "team"},
				{ID: "b", APIKey: "k2", Pool: "team"},
			}},
			wantErr: true,
		},
		{
			name: "standalone accounts unique keys ok",
			cfg: Config{Accounts: []AccountConfig{
				{ID: "a", APIKey: "k1"},
				{ID: "b", APIKey: "k2"},
			}},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRegistryAddExtendsPool(t *testing.T) {
	reg, _ := NewRegistry(nil)
	if err := reg.Add(poolAccount("a", "k", "team", idHandler("a"))); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	// Same key + same pool -> extends the pool.
	if err := reg.Add(poolAccount("b", "k", "team", idHandler("b"))); err != nil {
		t.Fatalf("Add b (extend pool): %v", err)
	}
	if reg.byKey["k"].Size() != 2 {
		t.Fatalf("pool size = %d, want 2", reg.byKey["k"].Size())
	}
	// Same key but different/empty pool -> rejected.
	if err := reg.Add(poolAccount("c", "k", "", idHandler("c"))); err == nil {
		t.Fatal("expected error adding incompatible account to pool key")
	}

	// Removing one member keeps the pool with the other.
	if _, err := reg.Remove("a"); err != nil {
		t.Fatalf("Remove a: %v", err)
	}
	if reg.byKey["k"].Size() != 1 {
		t.Fatalf("pool size after remove = %d, want 1", reg.byKey["k"].Size())
	}
	// Removing the last member deletes the pool/key.
	if _, err := reg.Remove("b"); err != nil {
		t.Fatalf("Remove b: %v", err)
	}
	if _, ok := reg.byKey["k"]; ok {
		t.Fatal("expected pool key k to be gone after removing all members")
	}
}
