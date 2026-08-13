package sse

import "net/http"

// BeginSSE sets the standard SSE response headers on w.
func BeginSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default (proxy_buffering on), which
	// holds back incrementally flushed SSE frames — including keep-alive pings —
	// until the buffer fills or the stream ends. This header disables that
	// buffering for nginx and compatible proxies; others ignore it.
	w.Header().Set("X-Accel-Buffering", "no")
}
