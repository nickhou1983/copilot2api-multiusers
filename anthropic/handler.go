package anthropic

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/whtsky/copilot2api/internal/models"
	"github.com/whtsky/copilot2api/internal/sse"
	"github.com/whtsky/copilot2api/internal/upstream"
)

// Handler handles Anthropic Messages API requests
type Handler struct {
	upstream *upstream.Client
	models   *models.Cache
}

// NewHandler creates a new Anthropic handler.
// The transport is used for upstream HTTP requests (pass nil to create a new one).
func NewHandler(authClient upstream.TokenProvider, transport *http.Transport, mc *models.Cache) *Handler {
	return &Handler{
		upstream: upstream.NewClient(authClient, transport),
		models:   mc,
	}
}

// ServeHTTP handles /v1/messages requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// /v1/messages/count_tokens is a distinct upstream endpoint; route it before
	// the standard Messages handling (which validates max_tokens that
	// count_tokens requests don't carry).
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		h.handleCountTokens(w, r)
		return
	}

	if r.Method != "POST" {
		WriteAnthropicError(w, http.StatusMethodNotAllowed, AnthropicErrorTypeInvalidRequest, "Method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, upstream.MaxRequestBody) // 10MB limit
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Parse Anthropic request
	var anthropicReq AnthropicMessagesRequest
	if err := json.Unmarshal(reqBody, &anthropicReq); err != nil {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate request
	if err := h.validateRequest(anthropicReq); err != nil {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// Resolve model alias (e.g. claude-haiku-4-5-20251001 -> claude-haiku-4.5)
	resolvedModel := resolveModelAlias(anthropicReq.Model)

	// Detect 1M context variant: Claude Code signals this via the anthropic-beta
	// header (e.g. "context-1m-2025-08-07"). Copilot historically exposed the 1M
	// variant as a separate "-1m" model ID, but newer models advertise a 1M
	// context window on the base ID directly. resolve1MContextModel only switches
	// to a "-1m" variant when the base model isn't already 1M and the variant
	// actually exists upstream, avoiding fabricated model IDs.
	if betaHeader := r.Header.Get("anthropic-beta"); betaHeader != "" && context1mRe.MatchString(betaHeader) {
		if infoMap, err := h.models.GetInfo(r.Context()); err == nil {
			if adjusted := resolve1MContextModel(resolvedModel, infoMap); adjusted != resolvedModel {
				slog.Debug("context-1m beta header: selected 1M variant", "from", resolvedModel, "to", adjusted)
				resolvedModel = adjusted
			} else {
				slog.Debug("context-1m beta header: base model already 1M or no variant", "model", resolvedModel)
			}
		}
	}

	modelChanged := resolvedModel != anthropicReq.Model
	if modelChanged {
		slog.Debug("resolved model alias", "from", anthropicReq.Model, "to", resolvedModel)
		anthropicReq.Model = resolvedModel
	}

	route := "chat_completions" // default fallback
	defer func() {
		slog.Info("anthropic request", "endpoint", "/v1/messages", "model", anthropicReq.Model, "stream", anthropicReq.Stream, "messages", len(anthropicReq.Messages), "route", route, "duration_ms", time.Since(start).Milliseconds())
	}()

	modelInfo, capabilityFetchFailed := h.getModelInfo(r.Context(), anthropicReq.Model)

	if modelSupportsEndpoint(modelInfo, "/v1/messages") {
		route = "native"
		cacheControlInfo := inspectCacheControl(reqBody)
		topLevelInfo := inspectTopLevelFields(reqBody)
		computerUseBetas := extractComputerUseBetas(r.Header.Values("anthropic-beta"))
		slog.Debug("native /messages passthrough request", "model", anthropicReq.Model, "top_level_keys", topLevelInfo.Keys, "has_context_management", topLevelInfo.HasContextManagement, "computer_use_betas", computerUseBetas, "cache_control_count", cacheControlInfo.Count, "cache_control_scope_count", cacheControlInfo.ScopeCount, "cache_control_paths", cacheControlInfo.Paths, "cache_control_scope_paths", cacheControlInfo.ScopePaths)
		// Only re-encode the body for native passthrough (the only path that
		// sends raw reqBody). Responses and Chat Completions paths use the
		// parsed struct, so they skip this JSON round-trip. context_management
		// is forwarded as-is (see normalizeNativeMessagesBody); the only body
		// mutations are the model alias and removing the upstream-rejected
		// cache_control.scope field.
		if modelChanged || cacheControlInfo.ScopeCount > 0 {
			newBody, err := normalizeNativeMessagesBody(reqBody, resolvedModel, modelChanged)
			if err != nil {
				WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid JSON: %v", err))
				return
			}
			if cacheControlInfo.ScopeCount > 0 {
				slog.Debug("normalized native /messages request", "removed_cache_control_scope_paths", cacheControlInfo.ScopePaths)
			}
			reqBody = newBody
		}
		h.handleNativeMessagesPassthrough(w, r, reqBody, anthropicReq.Stream, topLevelInfo.HasContextManagement, computerUseBetas)
		return
	}

	// Route based on model capabilities
	if modelSupportsEndpoint(modelInfo, "/responses") {
		route = "responses"
		h.handleViaResponsesAPI(w, r, anthropicReq)
		return
	}

	if capabilityFetchFailed {
		slog.Warn("failed to fetch model capabilities, falling back to Chat Completions", "model", anthropicReq.Model)
	}

	h.handleViaChatCompletions(w, r, anthropicReq)
}

func (h *Handler) validateRequest(req AnthropicMessagesRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}

	if req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}

	if len(req.Messages) == 0 && req.System == nil {
		return fmt.Errorf("either messages or system must be provided")
	}

	return nil
}

func (h *Handler) handleNativeMessagesPassthrough(w http.ResponseWriter, r *http.Request, body []byte, stream bool, hasContextManagement bool, computerUseBetas []string) {
	extraHeaders := buildUpstreamBetaHeaders(collectUpstreamBetas(hasContextManagement, computerUseBetas))
	if stream {
		resp, _, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/v1/messages", Body: body, Stream: true, QueryString: r.URL.RawQuery, ExtraHeaders: extraHeaders})
		if err != nil {
			var upstreamErr *upstream.UpstreamError
			if errors.As(err, &upstreamErr) {
				h.writeRawUpstreamError(w, upstreamErr)
				return
			}
			upstream.LogRequestError("native /messages streaming request failed", err)
			sse.BeginSSE(w)
			w.WriteHeader(http.StatusBadGateway)
			h.writeSSEError(w, "Upstream streaming request failed")
			return
		}
		defer resp.Body.Close()

		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Streaming unsupported")
			return
		}

		sse.BeginSSE(w)

		reader := bufio.NewReaderSize(resp.Body, 32*1024)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				if _, writeErr := w.Write(line); writeErr != nil {
					slog.Error("failed to write native /messages stream", "error", writeErr)
					return
				}
				// Flush at SSE event boundaries (blank lines) instead of every line
				// to reduce syscall overhead while maintaining correct SSE delivery.
				if isBlankSSELine(line) {
					flusher.Flush()
				}
			}

			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				slog.Error("error reading native /messages stream", "error", err)
				return
			}
		}

		return
	}

	_, respData, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/v1/messages", Body: body, QueryString: r.URL.RawQuery, ExtraHeaders: extraHeaders})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			h.writeRawUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("native /messages request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream request failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respData)
}

// --- Chat Completions path (existing fallback) ---

func (h *Handler) handleViaChatCompletions(w http.ResponseWriter, r *http.Request, anthropicReq AnthropicMessagesRequest) {
	openAIReq, err := ConvertAnthropicToOpenAI(anthropicReq)
	if err != nil {
		slog.Error("failed to convert Anthropic request to OpenAI", "error", err)
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Failed to convert request: %v", err))
		return
	}

	if anthropicReq.Stream {
		h.handleStreamingRequest(w, r, openAIReq)
	} else {
		h.handleNonStreamingRequest(w, r, openAIReq)
	}
}

func (h *Handler) handleNonStreamingRequest(w http.ResponseWriter, r *http.Request, openAIReq OpenAIChatCompletionsRequest) {
	openAIReq.Stream = false
	_, respData, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/chat/completions", Body: openAIReq})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			h.handleUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("upstream request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream request failed")
		return
	}

	slog.Debug("chat completions response", "size", len(respData))

	var openAIResp OpenAIChatCompletionsResponse
	if err := json.Unmarshal(respData, &openAIResp); err != nil {
		slog.Error("failed to parse OpenAI response", "error", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Failed to parse upstream response")
		return
	}

	anthropicResp, err := ConvertOpenAIToAnthropic(openAIResp)
	if err != nil {
		slog.Error("failed to convert OpenAI response to Anthropic", "error", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Failed to convert response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *Handler) handleStreamingRequest(w http.ResponseWriter, r *http.Request, openAIReq OpenAIChatCompletionsRequest) {
	openAIReq.Stream = true
	resp, _, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/chat/completions", Body: openAIReq, Stream: true})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			h.handleUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("upstream streaming request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream streaming request failed")
		return
	}
	defer resp.Body.Close()

	state := NewStreamState()

	finished := h.streamSSE(w, resp.Body, func(event *upstreamSSEEvent) ([]AnthropicStreamEvent, bool, error) {
		var chunk OpenAIChatCompletionChunk
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			slog.Warn("failed to parse OpenAI chunk", "error", err, "data", truncate(event.Data, 200))
			return nil, false, nil
		}

		events, err := ConvertOpenAIChunkToAnthropicEvents(chunk, state)
		if err != nil {
			return nil, false, err
		}

		return events, state.Finished, nil
	})

	if !finished {
		slog.Warn("chat completions stream ended without finish event")
	}
}

// --- Responses API path ---

func (h *Handler) handleViaResponsesAPI(w http.ResponseWriter, r *http.Request, anthropicReq AnthropicMessagesRequest) {
	responsesReq, err := ConvertAnthropicToResponses(anthropicReq)
	if err != nil {
		slog.Error("failed to convert Anthropic request to Responses", "error", err)
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Failed to convert request: %v", err))
		return
	}

	slog.Debug("responses request", "model", responsesReq.Model, "input_items", len(responsesReq.Input), "stream", responsesReq.Stream)

	if anthropicReq.Stream {
		h.handleResponsesStreaming(w, r, responsesReq)
	} else {
		h.handleResponsesNonStreaming(w, r, responsesReq)
	}
}

func (h *Handler) handleResponsesNonStreaming(w http.ResponseWriter, r *http.Request, responsesReq ResponsesRequest) {
	responsesReq.Stream = false
	_, respData, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/responses", Body: responsesReq})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			slog.Debug("responses upstream error", "status", upstreamErr.StatusCode, "body", truncate(string(upstreamErr.Body), 500))
			h.handleUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("responses upstream request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream request failed")
		return
	}

	slog.Debug("responses result", "size", len(respData))

	var result ResponsesResult
	if err := json.Unmarshal(respData, &result); err != nil {
		slog.Error("failed to parse Responses result", "error", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Failed to parse upstream response")
		return
	}

	anthropicResp := ConvertResponsesToAnthropic(result)
	slog.Debug("translated anthropic response", "id", anthropicResp.ID, "stop_reason", anthropicResp.StopReason, "content_blocks", len(anthropicResp.Content))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *Handler) handleResponsesStreaming(w http.ResponseWriter, r *http.Request, responsesReq ResponsesRequest) {
	responsesReq.Stream = true
	resp, _, err := h.upstream.Do(r.Context(), upstream.Request{Endpoint: "/responses", Body: responsesReq, Stream: true})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			slog.Debug("responses streaming upstream error", "status", upstreamErr.StatusCode, "body", truncate(string(upstreamErr.Body), 500))
			h.handleUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("responses streaming request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream streaming request failed")
		return
	}
	defer resp.Body.Close()

	state := NewResponsesStreamState()

	finished := h.streamSSE(w, resp.Body, func(event *upstreamSSEEvent) ([]AnthropicStreamEvent, bool, error) {
		var streamEvent ResponseStreamEvent
		if err := json.Unmarshal([]byte(event.Data), &streamEvent); err != nil {
			slog.Debug("failed to parse Responses stream event", "error", err, "data", truncate(event.Data, 200), "event", event.Event)
			return nil, false, nil
		}
		if streamEvent.Type == "" && event.Event != "" {
			streamEvent.Type = event.Event
		}

		events := TranslateResponsesStreamEvent(streamEvent, state)

		slog.Debug("responses stream event translated", "type", streamEvent.Type, "output_events", len(events))

		return events, state.MessageCompleted, nil
	})

	if finished {
		slog.Debug("responses stream completed")
	} else {
		slog.Warn("responses stream ended without completion")
	}
}

// sseTranslator translates a raw upstream SSE event into Anthropic stream
// events. It returns the translated events, whether the stream is logically
// complete (done=true), and any fatal error that should abort the stream.
// Returning (nil, false, nil) skips the event silently.
type sseTranslator func(event *upstreamSSEEvent) (events []AnthropicStreamEvent, done bool, err error)

// streamSSE is the shared SSE read-translate-write loop used by both the Chat
// Completions and Responses streaming paths. It sets the SSE response headers,
// reads upstream SSE events, passes each one to translate, writes the resulting
// Anthropic events, and flushes. It returns true when the translator signals
// completion, false otherwise (EOF / read error / write error).
func (h *Handler) streamSSE(w http.ResponseWriter, body io.Reader, translate sseTranslator) (finished bool) {
	sse.BeginSSE(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Streaming unsupported")
		return false
	}

	reader := bufio.NewReaderSize(body, 32*1024)

	for {
		sseEvent, readErr := readSSEEvent(reader)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			slog.Error("error reading streaming response", "error", readErr)
			h.writeSSEError(w, "Error reading streaming response")
			return false
		}
		if sseEvent == nil {
			continue
		}

		dataStr := strings.TrimSpace(sseEvent.Data)
		if dataStr == "" {
			continue
		}
		if dataStr == "[DONE]" {
			break
		}
		sseEvent.Data = dataStr // pass pre-trimmed data to translator

		events, done, err := translate(sseEvent)
		if err != nil {
			slog.Error("failed to translate streaming event", "error", err)
			h.writeSSEError(w, "Failed to convert streaming response")
			return false
		}

		for _, event := range events {
			if err := h.writeSSEEvent(w, event); err != nil {
				slog.Error("failed to write SSE event", "error", err)
				return false
			}
		}
		if len(events) > 0 {
			flusher.Flush()
		}

		if done {
			return true
		}
	}

	// Stream ended without the translator signalling completion.
	h.writeSSEError(w, "Stream ended unexpectedly without completion")
	h.writeSSEEvent(w, AnthropicStreamEvent{Type: "message_stop"})
	flusher.Flush()
	return false
}

type upstreamSSEEvent struct {
	Event string
	Data  string
}

const maxSSELineSize = 1 << 20 // 1MB

// errSSELineTooLong is returned when a single SSE line exceeds maxSSELineSize.
var errSSELineTooLong = fmt.Errorf("SSE line exceeds %d bytes", maxSSELineSize)

func readSSEEvent(reader *bufio.Reader) (*upstreamSSEEvent, error) {
	var (
		eventType string
		dataLines []string
	)

	for {
		line, err := readLimitedLine(reader, maxSSELineSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if eventType == "" && len(dataLines) == 0 {
				if errors.Is(err, io.EOF) {
					return nil, io.EOF
				}
				continue
			}
			return &upstreamSSEEvent{
				Event: eventType,
				Data:  strings.Join(dataLines, "\n"),
			}, nil
		}

		if !strings.HasPrefix(line, ":") {
			field, value, found := strings.Cut(line, ":")
			if !found {
				field = line
				value = ""
			} else {
				value = strings.TrimPrefix(value, " ")
			}

			switch field {
			case "event":
				eventType = value
			case "data":
				dataLines = append(dataLines, value)
			}
		}

		if errors.Is(err, io.EOF) {
			if eventType == "" && len(dataLines) == 0 {
				return nil, io.EOF
			}
			return &upstreamSSEEvent{
				Event: eventType,
				Data:  strings.Join(dataLines, "\n"),
			}, nil
		}
	}
}

// readLimitedLine reads a line (up to and including '\n') from reader.
// It uses ReadSlice for efficiency and falls back to accumulation when
// the line spans multiple buffer fills, returning errSSELineTooLong if
// the accumulated length exceeds maxLen.
func readLimitedLine(reader *bufio.Reader, maxLen int) (string, error) {
	slice, err := reader.ReadSlice('\n')
	if err == nil {
		// Common fast path: full line fit in the buffer.
		if len(slice) > maxLen {
			return "", errSSELineTooLong
		}
		return string(slice), nil
	}
	if err != bufio.ErrBufferFull {
		// io.EOF or other real error — return whatever was read.
		if len(slice) > maxLen {
			return "", errSSELineTooLong
		}
		return string(slice), err
	}
	// Line is longer than the internal buffer — accumulate chunks.
	var buf strings.Builder
	buf.Write(slice)
	for {
		if buf.Len() > maxLen {
			return "", errSSELineTooLong
		}
		slice, err = reader.ReadSlice('\n')
		buf.Write(slice)
		if err != bufio.ErrBufferFull {
			if buf.Len() > maxLen {
				return "", errSSELineTooLong
			}
			return buf.String(), err
		}
	}
}

// --- SSE helpers ---

func (h *Handler) writeSSEEvent(w io.Writer, event AnthropicStreamEvent) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, eventData)
	return err
}

func (h *Handler) writeSSEError(w io.Writer, message string) {
	h.writeSSETypedError(w, AnthropicErrorTypeAPI, message)
}

func (h *Handler) writeSSETypedError(w io.Writer, errorType, message string) {
	errorEvent := CreateErrorEvent(message)
	if errorEvent.Error != nil {
		errorEvent.Error.Type = errorType
	}
	h.writeSSEEvent(w, errorEvent)
}

func mapOpenAIErrorTypeToAnthropic(errorType string) string {
	switch errorType {
	case "invalid_request_error":
		return AnthropicErrorTypeInvalidRequest
	case "authentication_error", "invalid_api_key":
		return AnthropicErrorTypeAuthentication
	case "permission_error", "insufficient_quota":
		return AnthropicErrorTypePermission
	case "not_found":
		return AnthropicErrorTypeNotFound
	case "rate_limit_exceeded":
		return AnthropicErrorTypeRateLimit
	case "overloaded":
		return AnthropicErrorTypeOverloaded
	default:
		return AnthropicErrorTypeAPI
	}
}

func mapStatusToAnthropicError(statusCode int) (string, string) {
	switch statusCode {
	case http.StatusBadRequest:
		return AnthropicErrorTypeInvalidRequest, "Invalid request"
	case http.StatusUnauthorized:
		return AnthropicErrorTypeAuthentication, "Authentication failed"
	case http.StatusForbidden:
		return AnthropicErrorTypePermission, "Permission denied"
	case http.StatusNotFound:
		return AnthropicErrorTypeNotFound, "Resource not found"
	case http.StatusTooManyRequests:
		return AnthropicErrorTypeRateLimit, "Rate limit exceeded"
	case http.StatusServiceUnavailable:
		return AnthropicErrorTypeOverloaded, "Service temporarily unavailable"
	default:
		return AnthropicErrorTypeAPI, "Internal error"
	}
}

func (h *Handler) mapUpstreamError(upstreamErr *upstream.UpstreamError) (string, string) {
	var openAIError struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	anthropicErrorType, message := mapStatusToAnthropicError(upstreamErr.StatusCode)

	if err := json.Unmarshal(upstreamErr.Body, &openAIError); err == nil && openAIError.Error.Message != "" {
		message = openAIError.Error.Message
		anthropicErrorType = mapOpenAIErrorTypeToAnthropic(openAIError.Error.Type)
	}

	return anthropicErrorType, message
}

func (h *Handler) writeRawUpstreamError(w http.ResponseWriter, upstreamErr *upstream.UpstreamError) {
	upstreamErr.WriteRawError(w)
}

// handleUpstreamError converts upstream OpenAI errors to Anthropic format
func (h *Handler) handleUpstreamError(w http.ResponseWriter, upstreamErr *upstream.UpstreamError) {
	anthropicErrorType, message := h.mapUpstreamError(upstreamErr)
	WriteAnthropicError(w, upstreamErr.StatusCode, anthropicErrorType, message)
}

// replaceModelInBody replaces the top-level "model" key in a JSON body via a
// targeted round-trip, preserving all other fields unchanged.
func replaceModelInBody(body []byte, newModel string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	modelJSON, _ := json.Marshal(newModel)
	raw["model"] = modelJSON
	return json.Marshal(raw)
}

// normalizeNativeMessagesBody applies the minimal mutations the upstream
// requires for native /v1/messages passthrough: optionally replacing the model
// alias and removing the cache_control.scope field that upstream rejects.
// context_management is intentionally preserved and forwarded as-is.
func normalizeNativeMessagesBody(body []byte, newModel string, replaceModel bool) ([]byte, error) {
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("request body must be a JSON object")
	}

	if replaceModel {
		obj["model"] = newModel
	}

	stripCacheControlScope(obj)
	return json.Marshal(obj)
}

// contextManagementBeta is the anthropic-beta token that activates the
// context_management feature upstream. The proxy adds it automatically when a
// request carries a context_management field so the feature is honored without
// the client having to set the header (which the proxy does not blindly
// forward).
const contextManagementBeta = "context-management-2025-06-27"

// extractComputerUseBetas returns the computer-use-* tokens found across the
// given client anthropic-beta header values, in order and de-duplicated. Each
// value is split on commas and trimmed, then matched against the anchored
// computerUseBetaRe so only standalone computer-use tokens are forwarded (never
// a substring of a larger token). The proxy does not blindly forward client
// beta headers on the native route, but it does allow computer-use tokens
// through so the computer use tool types are recognized upstream (the upstream
// itself supports computer use; only the beta header was being stripped). Any
// non computer-use tokens are ignored.
func extractComputerUseBetas(betaHeaderValues []string) []string {
	if len(betaHeaderValues) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, hv := range betaHeaderValues {
		for _, tok := range strings.Split(hv, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" || !computerUseBetaRe.MatchString(tok) {
				continue
			}
			if _, dup := seen[tok]; dup {
				continue
			}
			seen[tok] = struct{}{}
			out = append(out, tok)
		}
	}
	return out
}

// buildUpstreamBetaHeaders merges the given anthropic-beta tokens into a single
// ExtraHeaders map for an upstream native request. The upstream client applies
// ExtraHeaders with a plain header Set (overwrite), so all beta tokens must be
// combined into one comma-separated "anthropic-beta" value rather than set
// separately. Tokens are de-duplicated in order; an empty result returns nil so
// no header is attached.
func buildUpstreamBetaHeaders(betas []string) map[string]string {
	if len(betas) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(betas))
	ordered := make([]string, 0, len(betas))
	for _, b := range betas {
		if b == "" {
			continue
		}
		if _, dup := seen[b]; dup {
			continue
		}
		seen[b] = struct{}{}
		ordered = append(ordered, b)
	}
	if len(ordered) == 0 {
		return nil
	}
	return map[string]string{"anthropic-beta": strings.Join(ordered, ",")}
}

// collectUpstreamBetas assembles the anthropic-beta tokens the proxy attaches to
// an upstream native request: the context-management beta (added automatically
// when the body carries a context_management field) plus any computer-use
// tokens forwarded from the client's anthropic-beta header.
func collectUpstreamBetas(hasContextManagement bool, computerUseBetas []string) []string {
	var betas []string
	if hasContextManagement {
		betas = append(betas, contextManagementBeta)
	}
	betas = append(betas, computerUseBetas...)
	return betas
}

// handleCountTokens proxies POST /v1/messages/count_tokens to the upstream
// Anthropic-compatible token counting endpoint. It resolves the model alias and
// strips the upstream-rejected cache_control.scope field, mirroring the native
// /v1/messages passthrough, then returns the upstream JSON response verbatim.
func (h *Handler) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != "POST" {
		WriteAnthropicError(w, http.StatusMethodNotAllowed, AnthropicErrorTypeInvalidRequest, "Method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, upstream.MaxRequestBody)
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(reqBody, &probe); err != nil {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}
	if probe.Model == "" {
		WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, "model is required")
		return
	}

	resolvedModel := resolveModelAlias(probe.Model)
	modelChanged := resolvedModel != probe.Model
	cacheControlInfo := inspectCacheControl(reqBody)
	topLevelInfo := inspectTopLevelFields(reqBody)

	defer func() {
		slog.Info("anthropic request", "endpoint", "/v1/messages/count_tokens", "model", resolvedModel, "route", "native", "duration_ms", time.Since(start).Milliseconds())
	}()

	body := reqBody
	if modelChanged || cacheControlInfo.ScopeCount > 0 {
		newBody, err := normalizeNativeMessagesBody(reqBody, resolvedModel, modelChanged)
		if err != nil {
			WriteAnthropicError(w, http.StatusBadRequest, AnthropicErrorTypeInvalidRequest, fmt.Sprintf("Invalid JSON: %v", err))
			return
		}
		body = newBody
	}

	_, respData, err := h.upstream.Do(r.Context(), upstream.Request{
		Endpoint:    "/v1/messages/count_tokens",
		Body:        body,
		QueryString: r.URL.RawQuery,
		ExtraHeaders: buildUpstreamBetaHeaders(collectUpstreamBetas(
			topLevelInfo.HasContextManagement,
			extractComputerUseBetas(r.Header.Values("anthropic-beta")),
		)),
	})
	if err != nil {
		var upstreamErr *upstream.UpstreamError
		if errors.As(err, &upstreamErr) {
			h.writeRawUpstreamError(w, upstreamErr)
			return
		}
		upstream.LogRequestError("native /messages/count_tokens request failed", err)
		WriteAnthropicError(w, http.StatusInternalServerError, AnthropicErrorTypeAPI, "Upstream request failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respData)
}

type topLevelFieldInspection struct {
	Keys                 []string
	HasContextManagement bool
}

func inspectTopLevelFields(body []byte) topLevelFieldInspection {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return topLevelFieldInspection{}
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	_, hasContextManagement := raw["context_management"]
	return topLevelFieldInspection{Keys: keys, HasContextManagement: hasContextManagement}
}

type cacheControlInspection struct {
	Count      int
	ScopeCount int
	Paths      []string
	ScopePaths []string
}

func inspectCacheControl(body []byte) cacheControlInspection {
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return cacheControlInspection{}
	}

	result := cacheControlInspection{}
	inspectCacheControlValue(raw, "$", &result)
	return result
}

func inspectCacheControlValue(v interface{}, path string, result *cacheControlInspection) {
	switch node := v.(type) {
	case map[string]interface{}:
		if cacheControl, ok := node["cache_control"].(map[string]interface{}); ok {
			result.Count++
			result.Paths = append(result.Paths, path+".cache_control")
			if _, hasScope := cacheControl["scope"]; hasScope {
				result.ScopeCount++
				result.ScopePaths = append(result.ScopePaths, path+".cache_control.scope")
			}
		}
		for key, child := range node {
			inspectCacheControlValue(child, path+"."+key, result)
		}
	case []interface{}:
		for i, child := range node {
			inspectCacheControlValue(child, fmt.Sprintf("%s[%d]", path, i), result)
		}
	}
}

func stripCacheControlScope(v interface{}) {
	switch node := v.(type) {
	case map[string]interface{}:
		if cacheControl, ok := node["cache_control"].(map[string]interface{}); ok {
			delete(cacheControl, "scope")
		}
		for _, child := range node {
			stripCacheControlScope(child)
		}
	case []interface{}:
		for _, child := range node {
			stripCacheControlScope(child)
		}
	}
}

// isBlankSSELine reports whether line consists solely of newline characters
// (\r and/or \n). SSE uses blank lines to delimit events; flushing only at
// these boundaries reduces syscall overhead while keeping delivery prompt.
func isBlankSSELine(line []byte) bool {
	for _, b := range line {
		if b != '\r' && b != '\n' {
			return false
		}
	}
	return len(line) > 0
}

// truncate limits a string to maxLen characters for debug logging.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// WriteAnthropicError writes an error response in Anthropic API format
func WriteAnthropicError(w http.ResponseWriter, statusCode int, errorType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := AnthropicErrorResponse{
		Type: "error",
		Error: AnthropicError{
			Type:    errorType,
			Message: message,
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}
