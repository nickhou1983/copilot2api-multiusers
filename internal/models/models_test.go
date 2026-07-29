package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whtsky/copilot2api/internal/copilot"
	"github.com/whtsky/copilot2api/internal/upstream"
)

func TestPickEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		info      *Info
		preferred []string
		want      string
	}{
		{
			name:      "nil info returns empty",
			info:      nil,
			preferred: []string{"/chat/completions"},
			want:      "",
		},
		{
			name:      "model supports first preferred",
			info:      &Info{ID: "gpt-4", SupportedEndpoints: []string{"/v1/chat/completions", "/v1/responses"}},
			preferred: []string{"/chat/completions", "/responses"},
			want:      "/chat/completions",
		},
		{
			name:      "model supports second preferred",
			info:      &Info{ID: "o3-mini", SupportedEndpoints: []string{"/v1/responses"}},
			preferred: []string{"/chat/completions", "/responses"},
			want:      "/responses",
		},
		{
			name:      "model supports neither",
			info:      &Info{ID: "embedding-model", SupportedEndpoints: []string{"/v1/embeddings"}},
			preferred: []string{"/chat/completions", "/responses"},
			want:      "",
		},
		{
			name:      "empty preferred list",
			info:      &Info{ID: "gpt-4", SupportedEndpoints: []string{"/v1/chat/completions"}},
			preferred: []string{},
			want:      "",
		},
		{
			name:      "normalizes /v1 prefix in preferred",
			info:      &Info{ID: "gpt-4", SupportedEndpoints: []string{"/v1/chat/completions"}},
			preferred: []string{"/v1/chat/completions"},
			want:      "/v1/chat/completions",
		},
		{
			name:      "normalizes no prefix in supported endpoints",
			info:      &Info{ID: "gpt-4", SupportedEndpoints: []string{"/chat/completions"}},
			preferred: []string{"/v1/chat/completions"},
			want:      "/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PickEndpoint(tt.info, tt.preferred)
			if got != tt.want {
				t.Errorf("PickEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSupportsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		info     *Info
		endpoint string
		want     bool
	}{
		{
			name:     "nil info",
			info:     nil,
			endpoint: "/chat/completions",
			want:     false,
		},
		{
			name:     "exact match with /v1 prefix",
			info:     &Info{SupportedEndpoints: []string{"/v1/chat/completions"}},
			endpoint: "/v1/chat/completions",
			want:     true,
		},
		{
			name:     "match without /v1 prefix",
			info:     &Info{SupportedEndpoints: []string{"/v1/chat/completions"}},
			endpoint: "/chat/completions",
			want:     true,
		},
		{
			name:     "no match",
			info:     &Info{SupportedEndpoints: []string{"/v1/responses"}},
			endpoint: "/chat/completions",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsEndpoint(tt.info, tt.endpoint)
			if got != tt.want {
				t.Errorf("SupportsEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/v1/chat/completions", "/chat/completions"},
		{"/chat/completions", "/chat/completions"},
		{"chat/completions", "/chat/completions"},
		{"/v1/responses", "/responses"},
		{"", "/"},
		{"  /v1/chat/completions  ", "/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEndpoint(tt.input)
			if got != tt.want {
				t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

type stubTokenProvider struct{ baseURL string }

func (s *stubTokenProvider) GetToken(context.Context) (string, error) { return "test-token", nil }
func (s *stubTokenProvider) GetBaseURL() string                       { return s.baseURL }
func (s *stubTokenProvider) HeaderProfile() copilot.Profile           { return copilot.ProfileEditor }

// handlerRoundTripper serves requests in-process so tests need no real listener.
type handlerRoundTripper struct{ h http.Handler }

func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rt.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func newTestClient(h http.Handler) *upstream.Client {
	uc := upstream.NewClient(&stubTokenProvider{baseURL: "http://upstream.test"}, nil)
	uc.HTTPClient = &http.Client{Transport: handlerRoundTripper{h: h}}
	return uc
}

func TestCacheRefreshBypassesTTL(t *testing.T) {
	var fetches atomic.Int64
	uc := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5"},{"id":"gpt-6"}]}`))
	}))

	c := NewCache(uc, time.Hour)
	ctx := context.Background()

	info, err := c.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if len(info) != 1 {
		t.Fatalf("initial models = %d, want 1", len(info))
	}

	// Within TTL, GetRaw serves from cache without another fetch.
	if _, err := c.GetRaw(ctx); err != nil {
		t.Fatalf("GetRaw: %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("fetches after cached read = %d, want 1", got)
	}

	// Refresh must hit upstream despite the valid cache entry.
	raw, err := c.Refresh(ctx)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := fetches.Load(); got != 2 {
		t.Fatalf("fetches after refresh = %d, want 2", got)
	}
	if !strings.Contains(string(raw), "gpt-6") {
		t.Fatalf("refresh returned stale data: %s", raw)
	}

	// Subsequent reads serve the refreshed data from cache.
	info, err = c.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo after refresh: %v", err)
	}
	if len(info) != 2 {
		t.Fatalf("models after refresh = %d, want 2", len(info))
	}
	if got := fetches.Load(); got != 2 {
		t.Fatalf("fetches after post-refresh read = %d, want 2", got)
	}
}

func TestCacheRefreshErrorKeepsCache(t *testing.T) {
	var fetches atomic.Int64
	uc := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fetches.Add(1) > 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5"}]}`))
	}))

	c := NewCache(uc, time.Hour)
	ctx := context.Background()

	if _, err := c.GetRaw(ctx); err != nil {
		t.Fatalf("GetRaw: %v", err)
	}
	if _, err := c.Refresh(ctx); err == nil {
		t.Fatal("Refresh: want error, got nil")
	}
	// Cached data remains served after a failed refresh.
	raw, err := c.GetRaw(ctx)
	if err != nil {
		t.Fatalf("GetRaw after failed refresh: %v", err)
	}
	if !strings.Contains(string(raw), "gpt-5") {
		t.Fatalf("cache lost after failed refresh: %s", raw)
	}
}
