package anthropic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedBody emits pre-defined chunks, sleeping before each one to simulate
// upstream think time. A zero-length chunk list ends the stream with io.EOF.
type scriptedBody struct {
	chunks []scriptedChunk
	i      int
	rest   string
}

type scriptedChunk struct {
	delay time.Duration
	data  string
}

func (s *scriptedBody) Read(p []byte) (int, error) {
	if s.rest == "" {
		if s.i >= len(s.chunks) {
			return 0, io.EOF
		}
		c := s.chunks[s.i]
		s.i++
		if c.delay > 0 {
			time.Sleep(c.delay)
		}
		s.rest = c.data
	}
	n := copy(p, s.rest)
	s.rest = s.rest[n:]
	return n, nil
}

// recorder is a concurrency-safe io.Writer + http.Flusher for the pipe tests.
type recorder struct {
	mu     sync.Mutex
	buf    strings.Builder
	failAt int // when > 0, the Nth write (1-based) and all later ones fail
	writes int
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes++
	if r.failAt > 0 && r.writes >= r.failAt {
		return 0, fmt.Errorf("simulated client disconnect")
	}
	return r.buf.Write(p)
}

func (r *recorder) Flush() {}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// stripPings removes every keep-alive frame so the remainder can be compared
// byte for byte against the upstream payload.
func stripPings(s string) string {
	return strings.ReplaceAll(s, nativeKeepAliveFrame, "")
}

func countPings(s string) int {
	return strings.Count(s, nativeKeepAliveFrame)
}

// Case 1 + 2: pings appear during upstream silence, and removing them restores
// the upstream bytes exactly.
func TestPipeNativeStream_InjectsPingWhileIdle(t *testing.T) {
	upstreamPayload := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

	body := &scriptedBody{chunks: []scriptedChunk{
		{data: "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"},
		{delay: 250 * time.Millisecond, data: "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
	}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 50 * time.Millisecond}
	h.pipeNativeStream(context.Background(), rec, rec, body)

	got := rec.String()
	if n := countPings(got); n == 0 {
		t.Fatalf("expected at least one keep-alive ping during idle window, got none: %q", got)
	}
	if stripped := stripPings(got); stripped != upstreamPayload {
		t.Errorf("stream corrupted by keep-alive injection:\n got: %q\nwant: %q", stripped, upstreamPayload)
	}
}

// Case 3: the upstream stalls mid-event (an `event:` line with no terminating
// blank line). A ping must not be injected there, since it would override the
// pending event name and make clients drop the event silently.
func TestPipeNativeStream_NeverSplitsEvent(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{data: "event: content_block_delta\n"},
		{delay: 300 * time.Millisecond, data: "data: {\"index\":0}\n\n"},
	}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 50 * time.Millisecond}
	h.pipeNativeStream(context.Background(), rec, rec, body)

	got := rec.String()
	// The event must arrive intact and unsplit.
	const want = "event: content_block_delta\ndata: {\"index\":0}\n\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("event was split by keep-alive injection: %q", got)
	}

	// Verify with the repository's own SSE parser that the event survived.
	ev, err := readSSEEvent(bufio.NewReader(strings.NewReader(got)))
	if err != nil {
		t.Fatalf("readSSEEvent failed: %v", err)
	}
	if ev.Event != "content_block_delta" {
		t.Errorf("event name clobbered: got %q, want %q", ev.Event, "content_block_delta")
	}
	if ev.Data != "{\"index\":0}" {
		t.Errorf("event data corrupted: got %q", ev.Data)
	}
}

// Case 4: a continuously active stream resets the idle timer, so no ping is
// ever injected.
func TestPipeNativeStream_NoPingWhileActive(t *testing.T) {
	var chunks []scriptedChunk
	var want strings.Builder
	for i := 0; i < 10; i++ {
		frame := fmt.Sprintf("event: content_block_delta\ndata: {\"index\":%d}\n\n", i)
		chunks = append(chunks, scriptedChunk{delay: 20 * time.Millisecond, data: frame})
		want.WriteString(frame)
	}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 100 * time.Millisecond}
	h.pipeNativeStream(context.Background(), rec, rec, &scriptedBody{chunks: chunks})

	got := rec.String()
	if n := countPings(got); n != 0 {
		t.Errorf("expected no pings on an active stream, got %d", n)
	}
	if got != want.String() {
		t.Errorf("stream mismatch:\n got: %q\nwant: %q", got, want.String())
	}
}

// Case 5: interval 0 disables keep-alive entirely.
func TestPipeNativeStream_KeepAliveDisabled(t *testing.T) {
	const payload = "event: message_stop\ndata: {}\n\n"
	body := &scriptedBody{chunks: []scriptedChunk{{delay: 200 * time.Millisecond, data: payload}}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 0}
	h.pipeNativeStream(context.Background(), rec, rec, body)

	got := rec.String()
	if n := countPings(got); n != 0 {
		t.Errorf("keep-alive disabled but %d ping(s) injected", n)
	}
	if got != payload {
		t.Errorf("stream mismatch: got %q, want %q", got, payload)
	}
}

// Case 7: cancelling the request context aborts the stream promptly instead of
// draining the remaining upstream response.
func TestPipeNativeStream_ContextCancel(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{data: "event: message_start\ndata: {}\n\n"},
		{delay: 10 * time.Second, data: "event: message_stop\ndata: {}\n\n"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	rec := &recorder{}
	h := &Handler{keepAliveInterval: 50 * time.Millisecond}

	returned := make(chan time.Duration, 1)
	start := time.Now()
	go func() {
		h.pipeNativeStream(ctx, rec, rec, body)
		returned <- time.Since(start)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case d := <-returned:
		if d > 2*time.Second {
			t.Errorf("pipeNativeStream took %v to honour context cancellation", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pipeNativeStream did not return after context cancellation")
	}
}

// Case 7b: a failing write (client gone) terminates the stream, which is what
// makes keep-alive doubles as a liveness probe during upstream silence.
func TestPipeNativeStream_StopsOnWriteFailure(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{delay: 10 * time.Second, data: "event: message_stop\ndata: {}\n\n"},
	}}

	rec := &recorder{failAt: 1} // the first keep-alive write fails
	h := &Handler{keepAliveInterval: 50 * time.Millisecond}

	returned := make(chan struct{})
	go func() {
		h.pipeNativeStream(context.Background(), rec, rec, body)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("pipeNativeStream did not return after a failed keep-alive write")
	}
}

// Case 8: the reader goroutine must not leak once the stream is abandoned.
func TestPipeNativeStream_NoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	for i := 0; i < 20; i++ {
		body := &scriptedBody{chunks: []scriptedChunk{
			{data: "event: message_start\ndata: {}\n\n"},
			{delay: 5 * time.Second, data: "never delivered"},
		}}
		ctx, cancel := context.WithCancel(context.Background())
		rec := &recorder{}
		h := &Handler{keepAliveInterval: 20 * time.Millisecond}

		done := make(chan struct{})
		go func() {
			h.pipeNativeStream(ctx, rec, rec, body)
			close(done)
		}()
		time.Sleep(30 * time.Millisecond)
		cancel()
		<-done
	}

	// Reader goroutines unblock once the (real) upstream body is closed; here the
	// scripted body returns on its own, so allow a moment for them to drain.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("goroutine leak: before=%d after=%d", before, runtime.NumGoroutine())
}

// Case 6: response headers must reach the client immediately, without waiting
// for the first upstream byte. A ResponseRecorder cannot observe real header
// timing, so this uses a live server and a raw TCP client.
func TestNativeStream_HeadersFlushedImmediately(t *testing.T) {
	const upstreamDelay = 700 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		sseBeginForTest(w)
		flusher.Flush() // the fix under test

		body := &scriptedBody{chunks: []scriptedChunk{
			{delay: upstreamDelay, data: "event: message_stop\ndata: {}\n\n"},
		}}
		h := &Handler{keepAliveInterval: 0}
		h.pipeNativeStream(r.Context(), w, flusher, body)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", u.Host)
	start := time.Now()

	buf := make([]byte, 256)
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("reading response headers: %v", err)
	}
	elapsed := time.Since(start)

	if !strings.Contains(string(buf[:n]), "text/event-stream") {
		t.Errorf("expected SSE headers in first read, got %q", string(buf[:n]))
	}
	if elapsed >= upstreamDelay {
		t.Errorf("response headers withheld until the first upstream byte: %v (upstream delay %v)", elapsed, upstreamDelay)
	}
}

// sseBeginForTest mirrors sse.BeginSSE without importing it, keeping this test
// focused on flush timing rather than header contents.
func sseBeginForTest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// End-to-end over a real TCP connection: keep-alive frames must reach the client
// incrementally while the upstream is silent, rather than being buffered until
// the stream ends. The in-memory recorder cannot catch this because its Flush is
// a no-op.
func TestNativeStream_PingsReachClientIncrementally(t *testing.T) {
	const (
		interval      = 150 * time.Millisecond
		upstreamDelay = 800 * time.Millisecond
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		sseBeginForTest(w)
		flusher.Flush()

		body := &scriptedBody{chunks: []scriptedChunk{
			{delay: upstreamDelay, data: "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
		}}
		h := &Handler{keepAliveInterval: interval}
		h.pipeNativeStream(r.Context(), w, flusher, body)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	start := time.Now()
	var firstPing time.Duration
	pings := 0
	sawStop := false

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "event: ping":
			if pings == 0 {
				firstPing = time.Since(start)
			}
			pings++
		case "event: message_stop":
			sawStop = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading stream: %v", err)
	}

	if pings == 0 {
		t.Fatal("no keep-alive ping reached the client")
	}
	if !sawStop {
		t.Error("upstream message_stop event did not reach the client")
	}
	// A ping must arrive well before the upstream finally responds, proving the
	// frames are flushed as they are produced instead of buffered to the end.
	if firstPing >= upstreamDelay {
		t.Errorf("first ping arrived at %v, not before the upstream payload at %v", firstPing, upstreamDelay)
	}
}

func TestKeepAliveIntervalFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		set  bool
		want time.Duration
	}{
		{name: "unset uses default", want: defaultKeepAliveInterval},
		{name: "empty uses default", env: "", set: true, want: defaultKeepAliveInterval},
		{name: "zero disables", env: "0", set: true, want: 0},
		{name: "custom value", env: "30", set: true, want: 30 * time.Second},
		{name: "negative falls back", env: "-5", set: true, want: defaultKeepAliveInterval},
		{name: "garbage falls back", env: "abc", set: true, want: defaultKeepAliveInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(keepAliveEnvVar, tt.env)
			}
			if got := keepAliveIntervalFromEnv(); got != tt.want {
				t.Errorf("keepAliveIntervalFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxUpstreamIdleFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		set  bool
		want time.Duration
	}{
		{name: "unset uses default", want: defaultMaxUpstreamIdle},
		{name: "empty uses default", env: "", set: true, want: defaultMaxUpstreamIdle},
		{name: "zero disables", env: "0", set: true, want: 0},
		{name: "custom value", env: "90", set: true, want: 90 * time.Second},
		{name: "negative falls back", env: "-1", set: true, want: defaultMaxUpstreamIdle},
		{name: "garbage falls back", env: "nope", set: true, want: defaultMaxUpstreamIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(maxUpstreamIdleEnvVar, tt.env)
			}
			if got := maxUpstreamIdleFromEnv(); got != tt.want {
				t.Errorf("maxUpstreamIdleFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The silence ceiling must abort a wedged upstream instead of letting keep-alive
// ping it forever, and must tell the client why via a terminal `error` event.
func TestPipeNativeStream_AbortsOnUpstreamSilence(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{data: "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"},
		{delay: 10 * time.Second, data: "event: message_stop\ndata: {}\n\n"},
	}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 20 * time.Millisecond, maxUpstreamIdle: 200 * time.Millisecond}

	returned := make(chan time.Duration, 1)
	start := time.Now()
	go func() {
		h.pipeNativeStream(context.Background(), rec, rec, body)
		returned <- time.Since(start)
	}()

	select {
	case d := <-returned:
		if d > 3*time.Second {
			t.Errorf("silence ceiling took %v to fire, want ~200ms", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pipeNativeStream did not abort on upstream silence")
	}

	got := rec.String()
	if countPings(got) == 0 {
		t.Error("expected keep-alive pings before the ceiling fired")
	}
	if !strings.Contains(got, "event: error") {
		t.Fatalf("expected a terminal error event, got %q", got)
	}
	// message_stop after a truncated stream would make the SDK accumulator treat
	// an incomplete message as a successful one.
	if strings.Contains(got, "message_stop") {
		t.Errorf("error event must not be followed by message_stop: %q", got)
	}

	// The error must be a well-formed Anthropic SSE event.
	events := strings.Split(stripPings(got), "\n\n")
	last := events[len(events)-2] + "\n\n"
	ev, err := readSSEEvent(bufio.NewReader(strings.NewReader(last)))
	if err != nil {
		t.Fatalf("readSSEEvent on the error frame failed: %v (frame %q)", err, last)
	}
	if ev.Event != "error" {
		t.Errorf("got event %q, want %q", ev.Event, "error")
	}
	if !strings.Contains(ev.Data, AnthropicErrorTypeAPI) {
		t.Errorf("error payload missing type %q: %s", AnthropicErrorTypeAPI, ev.Data)
	}
}

// An active stream keeps resetting the ceiling, so a long but continuously
// producing stream is never aborted.
func TestPipeNativeStream_SilenceCeilingResetsOnActivity(t *testing.T) {
	var chunks []scriptedChunk
	var want strings.Builder
	for i := 0; i < 10; i++ {
		frame := fmt.Sprintf("event: content_block_delta\ndata: {\"index\":%d}\n\n", i)
		chunks = append(chunks, scriptedChunk{delay: 30 * time.Millisecond, data: frame})
		want.WriteString(frame)
	}

	rec := &recorder{}
	// Total runtime (~300ms) far exceeds the ceiling, but no single gap does.
	h := &Handler{keepAliveInterval: 0, maxUpstreamIdle: 120 * time.Millisecond}
	h.pipeNativeStream(context.Background(), rec, rec, &scriptedBody{chunks: chunks})

	got := rec.String()
	if strings.Contains(got, "event: error") {
		t.Errorf("silence ceiling fired on a continuously active stream: %q", got)
	}
	if got != want.String() {
		t.Errorf("stream mismatch:\n got: %q\nwant: %q", got, want.String())
	}
}

// The ceiling is an independent safety net: it must apply even when keep-alive
// is switched off.
func TestPipeNativeStream_SilenceCeilingWithoutKeepAlive(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{delay: 10 * time.Second, data: "event: message_stop\ndata: {}\n\n"},
	}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 0, maxUpstreamIdle: 150 * time.Millisecond}

	returned := make(chan struct{})
	go func() {
		h.pipeNativeStream(context.Background(), rec, rec, body)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("silence ceiling did not fire with keep-alive disabled")
	}

	got := rec.String()
	if countPings(got) != 0 {
		t.Errorf("keep-alive disabled but pings were injected: %q", got)
	}
	if !strings.Contains(got, "event: error") {
		t.Errorf("expected a terminal error event, got %q", got)
	}
}

// Zero disables the ceiling, preserving the previous unbounded behaviour.
func TestPipeNativeStream_SilenceCeilingDisabled(t *testing.T) {
	const payload = "event: message_stop\ndata: {}\n\n"
	body := &scriptedBody{chunks: []scriptedChunk{{delay: 250 * time.Millisecond, data: payload}}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 0, maxUpstreamIdle: 0}
	h.pipeNativeStream(context.Background(), rec, rec, body)

	if got := rec.String(); got != payload {
		t.Errorf("stream mismatch: got %q, want %q", got, payload)
	}
}

// A stall mid-event must not leave a dangling `event:` line glued to the error
// frame, which would clobber the error's own event name.
func TestPipeNativeStream_SilenceCeilingClosesPartialEvent(t *testing.T) {
	body := &scriptedBody{chunks: []scriptedChunk{
		{data: "event: content_block_delta\n"},
		{delay: 10 * time.Second, data: "data: {}\n\n"},
	}}

	rec := &recorder{}
	h := &Handler{keepAliveInterval: 0, maxUpstreamIdle: 150 * time.Millisecond}

	returned := make(chan struct{})
	go func() {
		h.pipeNativeStream(context.Background(), rec, rec, body)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("silence ceiling did not fire on a mid-event stall")
	}

	got := rec.String()
	// The truncated event is terminated before the error frame begins.
	if !strings.HasPrefix(got, "event: content_block_delta\n\nevent: error\n") {
		t.Fatalf("partial event was not closed before the error frame: %q", got)
	}

	reader := bufio.NewReader(strings.NewReader(got))
	// First frame: the truncated delta, which has no data and is dropped.
	if _, err := readSSEEvent(reader); err != nil {
		t.Fatalf("readSSEEvent on the truncated frame failed: %v", err)
	}
	ev, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("readSSEEvent on the error frame failed: %v", err)
	}
	if ev.Event != "error" {
		t.Errorf("error event name clobbered by the partial event: got %q", ev.Event)
	}
}
