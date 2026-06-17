package stats

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// maxScanLine caps a single buffered SSE line while scanning a stream for
// usage. A line exceeding this stops scanning (the request's tokens are then
// not counted) but never affects the bytes passed through to the client.
const maxScanLine = 1 << 20

// kind classifies an upstream endpoint by its usage/response format.
type kind int

const (
	kindNone      kind = iota
	kindOpenAI         // /chat/completions, /embeddings
	kindResponses      // /responses
	kindAnthropic      // /v1/messages
)

func endpointKind(endpoint string) kind {
	switch {
	case strings.HasPrefix(endpoint, "/v1/messages"):
		return kindAnthropic
	case strings.HasPrefix(endpoint, "/responses"):
		return kindResponses
	case strings.HasPrefix(endpoint, "/chat/completions"), strings.HasPrefix(endpoint, "/embeddings"):
		return kindOpenAI
	default:
		return kindNone
	}
}

// --- minimal local JSON shapes (no dependency on internal/types or anthropic,
// to avoid an import cycle: upstream -> stats must not pull those back in) ---

type openaiResp struct {
	Model string       `json:"model"`
	Usage *openaiUsage `json:"usage"`
}

type openaiUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type responsesResult struct {
	Model string          `json:"model"`
	Usage *responsesUsage `json:"usage"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type responsesStreamEvent struct {
	Type     string           `json:"type"`
	Response *responsesResult `json:"response"`
}

type anthropicMessage struct {
	Model string          `json:"model"`
	Usage *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type anthropicStreamEvent struct {
	Type    string            `json:"type"`
	Message *anthropicMessage `json:"message"` // message_start
	Usage   *anthropicUsage   `json:"usage"`   // message_delta
}

// --- normalization: uniform meaning where Input excludes Cached/CacheCreation ---

func clampNonNeg(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func normalizeOpenAI(u *openaiUsage) Usage {
	if u == nil {
		return Usage{}
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return Usage{
		Input:  clampNonNeg(u.PromptTokens - cached),
		Output: u.CompletionTokens,
		Cached: cached,
	}
}

func normalizeResponses(u *responsesUsage) Usage {
	if u == nil {
		return Usage{}
	}
	cached := 0
	if u.InputTokensDetails != nil {
		cached = u.InputTokensDetails.CachedTokens
	}
	return Usage{
		Input:  clampNonNeg(u.InputTokens - cached),
		Output: u.OutputTokens,
		Cached: cached,
	}
}

func normalizeAnthropic(u *anthropicUsage) Usage {
	if u == nil {
		return Usage{}
	}
	return Usage{
		Input:         u.InputTokens,
		Output:        u.OutputTokens,
		Cached:        u.CacheReadInputTokens,
		CacheCreation: u.CacheCreationInputTokens,
	}
}

// ParseUsage extracts the model and normalized usage from a non-streaming
// upstream response body. ok is false for endpoints without usage (e.g.
// /models) or unparseable bodies, signaling the caller to skip recording.
func ParseUsage(endpoint string, body []byte) (model string, u Usage, ok bool) {
	switch endpointKind(endpoint) {
	case kindOpenAI:
		var r openaiResp
		if err := json.Unmarshal(body, &r); err != nil {
			return "", Usage{}, false
		}
		return r.Model, normalizeOpenAI(r.Usage), true
	case kindResponses:
		var r responsesResult
		if err := json.Unmarshal(body, &r); err != nil {
			return "", Usage{}, false
		}
		return r.Model, normalizeResponses(r.Usage), true
	case kindAnthropic:
		var m anthropicMessage
		if err := json.Unmarshal(body, &m); err != nil {
			return "", Usage{}, false
		}
		return m.Model, normalizeAnthropic(m.Usage), true
	default:
		return "", Usage{}, false
	}
}

// usageScanner wraps a streaming response body: it passes every byte through to
// the reader unchanged while side-scanning the SSE stream for the final usage,
// which it records on Close. A successful stream always records once (request
// count), even when no usage was present (e.g. OpenAI streaming without
// stream_options.include_usage).
type usageScanner struct {
	rec  *Recorder
	kind kind
	rc   io.ReadCloser

	leftover []byte
	overflow bool // a line exceeded maxScanLine; stop scanning

	model  string
	usage  Usage
	closed bool
}

// NewUsageScanner wraps rc so that streaming usage for endpoint is recorded to
// rec when the body is closed. If rec is nil or the endpoint carries no usage,
// rc is returned unwrapped.
func NewUsageScanner(rec *Recorder, endpoint string, rc io.ReadCloser) io.ReadCloser {
	k := endpointKind(endpoint)
	if rec == nil || k == kindNone {
		return rc
	}
	return &usageScanner{rec: rec, kind: k, rc: rc}
}

func (s *usageScanner) Read(p []byte) (int, error) {
	n, err := s.rc.Read(p)
	if n > 0 && !s.overflow {
		s.scan(p[:n])
	}
	return n, err
}

func (s *usageScanner) scan(b []byte) {
	s.leftover = append(s.leftover, b...)
	for {
		i := bytes.IndexByte(s.leftover, '\n')
		if i < 0 {
			if len(s.leftover) > maxScanLine {
				s.overflow = true
				s.leftover = nil
			}
			return
		}
		line := s.leftover[:i]
		s.leftover = s.leftover[i+1:]
		s.processLine(line)
	}
}

func (s *usageScanner) processLine(line []byte) {
	line = bytes.TrimRight(line, "\r")
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	payload := bytes.TrimPrefix(line, []byte("data:"))
	payload = bytes.TrimPrefix(payload, []byte(" "))
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}

	switch s.kind {
	case kindOpenAI:
		var r openaiResp
		if json.Unmarshal(payload, &r) != nil {
			return
		}
		if r.Model != "" {
			s.model = r.Model
		}
		if r.Usage != nil {
			s.usage = normalizeOpenAI(r.Usage)
		}
	case kindResponses:
		var ev responsesStreamEvent
		if json.Unmarshal(payload, &ev) != nil {
			return
		}
		if ev.Response != nil {
			if ev.Response.Model != "" {
				s.model = ev.Response.Model
			}
			if ev.Response.Usage != nil {
				s.usage = normalizeResponses(ev.Response.Usage)
			}
		}
	case kindAnthropic:
		var ev anthropicStreamEvent
		if json.Unmarshal(payload, &ev) != nil {
			return
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				if ev.Message.Model != "" {
					s.model = ev.Message.Model
				}
				if ev.Message.Usage != nil {
					base := normalizeAnthropic(ev.Message.Usage)
					s.usage.Input = base.Input
					s.usage.Cached = base.Cached
					s.usage.CacheCreation = base.CacheCreation
				}
			}
		case "message_delta":
			if ev.Usage != nil {
				s.usage.Output = ev.Usage.OutputTokens
			}
		}
	}
}

func (s *usageScanner) Close() error {
	if !s.closed {
		s.closed = true
		s.rec.Record(s.model, s.usage)
	}
	return s.rc.Close()
}
