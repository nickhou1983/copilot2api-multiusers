package accounts

import (
	"bytes"
	"io"
	"net/http"
	"sync/atomic"
)

// maxPooledRetryBody caps how much of a request body is buffered in memory to
// enable failover retries across pool members. It mirrors
// upstream.MaxRequestBody (10MB); requests larger than this are served by the
// primary member only (no failover) since buffering them would be wasteful and
// the downstream handler enforces the same limit anyway.
const maxPooledRetryBody = 10 << 20

// Pool groups one or more accounts that share a single API key. Requests to
// that key are distributed across the members using round-robin selection, with
// automatic failover to the next member when an attempt fails with a retryable
// upstream status (rate limit, quota, or transient 5xx).
//
// A pool with a single member behaves exactly like the classic
// one-key-one-account mapping and skips all retry machinery.
type Pool struct {
	// Name is the human-readable pool label from config (empty for singletons).
	Name string
	// Key is the shared API key that routes to this pool.
	Key string

	members []*Account
	rr      atomic.Uint64
}

// newPool creates a pool from its members. members must be non-empty.
func newPool(key, name string, members []*Account) *Pool {
	return &Pool{Name: name, Key: key, members: members}
}

// Size returns the number of accounts in the pool.
func (p *Pool) Size() int { return len(p.members) }

// Members returns the pool's accounts (read-only; do not mutate).
func (p *Pool) Members() []*Account { return p.members }

// pick returns the next member by round-robin. It advances the cursor.
func (p *Pool) pick() *Account {
	n := len(p.members)
	if n == 1 {
		return p.members[0]
	}
	i := int(p.rr.Add(1)-1) % n
	return p.members[i]
}

// order returns the members ordered for a single request: the round-robin
// primary first, followed by the remaining members as failover candidates. It
// advances the cursor once so consecutive requests start on different members.
// The result is a fresh slice, safe to iterate without holding a lock.
func (p *Pool) order() []*Account {
	n := len(p.members)
	out := make([]*Account, 0, n)
	if n == 0 {
		return out
	}
	start := 0
	if n > 1 {
		start = int(p.rr.Add(1)-1) % n
	}
	for i := 0; i < n; i++ {
		out = append(out, p.members[(start+i)%n])
	}
	return out
}

// isRetryableStatus reports whether an upstream/handler status warrants trying
// the next pool member. It covers rate limiting, quota/billing, and transient
// server errors. A permission 403 is included because Copilot returns it when a
// plan/quota bars a model, which a different account may still serve.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429 rate limit
		http.StatusPaymentRequired,     // 402 quota/billing
		http.StatusForbidden,           // 403 plan/quota restriction
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// captureWriter wraps the real ResponseWriter for one pool attempt. It defers
// committing to the client until it sees the response status:
//   - a retryable status (and not the last attempt) is suppressed so the caller
//     can retry on the next member, discarding this attempt's output;
//   - any other status (including 200) commits immediately and streams through.
//
// Only the response of the committed attempt reaches the client, so failover is
// transparent. Once a 200 stream has started it is committed and cannot be
// retried — failover only helps for pre-stream errors, which is the intended
// use case.
type captureWriter struct {
	rw         http.ResponseWriter
	scratch    http.Header
	status     int
	headerDone bool
	committed  bool
	aborted    bool
	last       bool
}

func (c *captureWriter) Header() http.Header {
	if c.committed {
		return c.rw.Header()
	}
	if c.scratch == nil {
		c.scratch = make(http.Header)
	}
	return c.scratch
}

func (c *captureWriter) WriteHeader(code int) {
	if c.committed || c.aborted || c.headerDone {
		return
	}
	c.headerDone = true
	c.status = code
	if !c.last && isRetryableStatus(code) {
		c.aborted = true
		return
	}
	c.commit(code)
}

func (c *captureWriter) commit(code int) {
	dst := c.rw.Header()
	for k, v := range c.scratch {
		dst[k] = v
	}
	c.rw.WriteHeader(code)
	c.committed = true
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if c.aborted {
		// Swallow the failed attempt's body; the caller will retry.
		return len(b), nil
	}
	if !c.committed {
		if !c.headerDone {
			c.WriteHeader(http.StatusOK)
		}
		if c.aborted {
			return len(b), nil
		}
	}
	return c.rw.Write(b)
}

// Flush implements http.Flusher so streaming handlers keep working. A flush
// before any Write is how streaming handlers (e.g. SSE via BeginSSE, which only
// sets headers) push the response head to the client ahead of the first byte;
// mirror net/http and treat it as an implicit 200 commit so those headers are
// delivered immediately instead of being withheld until (or dropped when there
// is no) first body byte. Error paths call WriteHeader(status) before any flush,
// so they still abort and fail over correctly.
func (c *captureWriter) Flush() {
	if c.aborted {
		return
	}
	if !c.committed {
		if !c.headerDone {
			c.WriteHeader(http.StatusOK)
		}
		if !c.committed {
			return
		}
	}
	if f, ok := c.rw.(http.Flusher); ok {
		f.Flush()
	}
}

// drainForRetry reads the request body so it can be replayed for each attempt.
// It returns (body, true, nil) when the body was fully buffered, or
// (nil, false, nil) when the body is absent or too large to buffer (in which
// case r.Body is left usable for a single pass).
func drainForRetry(r *http.Request) ([]byte, bool, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, true, nil
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxPooledRetryBody+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(buf)) > maxPooledRetryBody {
		// Too large to buffer for retries: restore a single-pass body.
		r.Body = &readCloser{Reader: io.MultiReader(bytes.NewReader(buf), r.Body), closer: r.Body}
		return nil, false, nil
	}
	_ = r.Body.Close()
	return buf, true, nil
}

// resetBody points r.Body at a fresh reader over body for the next attempt.
func resetBody(r *http.Request, body []byte) {
	if body == nil {
		r.Body = http.NoBody
		r.ContentLength = 0
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
}

// readCloser adapts a Reader plus an underlying Closer into an io.ReadCloser.
type readCloser struct {
	io.Reader
	closer io.Closer
}

func (rc *readCloser) Close() error {
	if rc.closer != nil {
		return rc.closer.Close()
	}
	return nil
}
