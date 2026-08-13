package sse

import (
	"net/http/httptest"
	"testing"
)

func TestBeginSSE(t *testing.T) {
	want := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		"Connection":    "keep-alive",
		// Without this, nginx's default proxy_buffering holds back incrementally
		// flushed frames and silently defeats SSE keep-alive pings.
		"X-Accel-Buffering": "no",
	}

	rec := httptest.NewRecorder()
	BeginSSE(rec)

	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}
