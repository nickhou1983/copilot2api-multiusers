package apitest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// getEnvOrDefault returns the value of the environment variable or a default.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// skipIfNotIntegration skips the test if COPILOT2API_INTEGRATION is not set.
func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("COPILOT2API_INTEGRATION") == "" {
		t.Skip("skipping integration test; set COPILOT2API_INTEGRATION=1 to run")
	}
}

// anthropicRequest builds the JSON body for an Anthropic Messages API request
// with the web_search built-in tool.
func anthropicWebSearchRequest(model string, query string, stream bool) ([]byte, error) {
	req := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"stream":     stream,
		"tools": []map[string]interface{}{
			{
				"type": "web_search_20250305",
				"name": "web_search",
			},
		},
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": query,
			},
		},
	}
	return json.Marshal(req)
}

// TestWebSearchNonStreaming tests calling Claude with web_search tool via the proxy (non-streaming).
func TestWebSearchNonStreaming(t *testing.T) {
	skipIfNotIntegration(t)

	baseURL := getEnvOrDefault("COPILOT2API_TEST_URL", "http://127.0.0.1:7777")
	apiKey := getEnvOrDefault("COPILOT2API_TEST_API_KEY", "dummy")
	model := getEnvOrDefault("COPILOT2API_TEST_MODEL", "claude-sonnet-4-20250514")

	body, err := anthropicWebSearchRequest(model, "What is the latest technology news today? Please search the web.", false)
	if err != nil {
		t.Fatalf("failed to build request body: %v", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Role       string `json:"role"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("failed to parse response JSON: %v\nbody: %s", err, string(respBody))
	}

	// Basic validations
	if result.ID == "" {
		t.Error("response ID is empty")
	}
	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.StopReason == "" {
		t.Error("stop_reason is empty")
	}
	if len(result.Content) == 0 {
		t.Fatal("response has no content blocks")
	}

	// Log content blocks for debugging
	t.Logf("Model: %s", result.Model)
	t.Logf("Stop reason: %s", result.StopReason)
	t.Logf("Usage: input=%d, output=%d", result.Usage.InputTokens, result.Usage.OutputTokens)
	for i, block := range result.Content {
		switch block.Type {
		case "text":
			t.Logf("Content[%d] type=text, text=%s", i, truncate(block.Text, 200))
		case "tool_use":
			t.Logf("Content[%d] type=tool_use, name=%s, id=%s", i, block.Name, block.ID)
		case "web_search_tool_result":
			t.Logf("Content[%d] type=web_search_tool_result", i)
		default:
			t.Logf("Content[%d] type=%s", i, block.Type)
		}
	}

	// Verify we got meaningful content (either text or web search results)
	hasContent := false
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			hasContent = true
			break
		}
		if block.Type == "web_search_tool_result" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Error("response has no text content or web search results")
	}
}

// TestWebSearchStreaming tests calling Claude with web_search tool via the proxy (streaming).
func TestWebSearchStreaming(t *testing.T) {
	skipIfNotIntegration(t)

	baseURL := getEnvOrDefault("COPILOT2API_TEST_URL", "http://127.0.0.1:7777")
	apiKey := getEnvOrDefault("COPILOT2API_TEST_API_KEY", "dummy")
	model := getEnvOrDefault("COPILOT2API_TEST_MODEL", "claude-sonnet-4-20250514")

	body, err := anthropicWebSearchRequest(model, "What is GitHub Copilot? Search the web for the latest information.", true)
	if err != nil {
		t.Fatalf("failed to build request body: %v", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d; body: %s", resp.StatusCode, string(respBody))
	}

	// Verify it's an SSE stream
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %q", contentType)
	}

	// Read SSE events
	scanner := bufio.NewScanner(resp.Body)
	var (
		eventTypes     []string
		textContent    strings.Builder
		gotMessageStop bool
		currentEvent   string
	)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			eventTypes = append(eventTypes, currentEvent)
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			if currentEvent == "message_stop" {
				gotMessageStop = true
				continue
			}

			if currentEvent == "content_block_delta" {
				var delta struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if delta.Delta.Type == "text_delta" {
						textContent.WriteString(delta.Delta.Text)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading SSE stream: %v", err)
	}

	// Validate stream
	if !gotMessageStop {
		t.Error("stream did not end with message_stop event")
	}

	if len(eventTypes) == 0 {
		t.Fatal("no SSE events received")
	}

	// Log stream info
	t.Logf("Received %d SSE events", len(eventTypes))
	t.Logf("Event types: %v", uniqueStrings(eventTypes))
	if textContent.Len() > 0 {
		t.Logf("Text content (truncated): %s", truncate(textContent.String(), 300))
	}

	// Verify we got the expected event flow
	hasMessageStart := false
	hasContentBlock := false
	for _, et := range eventTypes {
		if et == "message_start" {
			hasMessageStart = true
		}
		if et == "content_block_start" || et == "content_block_delta" {
			hasContentBlock = true
		}
	}
	if !hasMessageStart {
		t.Error("stream missing message_start event")
	}
	if !hasContentBlock {
		t.Error("stream missing content_block events")
	}
}

// truncate returns at most n characters of s.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("... (%d chars total)", len(s))
}

// uniqueStrings returns unique strings preserving order.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
