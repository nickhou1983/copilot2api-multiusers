package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestUnmarshalSearchResultBlock verifies a request carrying a search_result
// content block (whose `source` is a bare string) parses without error. This
// is the regression guard for the proxy previously rejecting such requests with
// "content must be string or array of blocks", which the Copilot upstream
// otherwise accepts (returning search_result_location citations).
func TestUnmarshalSearchResultBlock(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4.6",
		"max_tokens": 100,
		"messages": [{
			"role": "user",
			"content": [
				{
					"type": "search_result",
					"source": "https://example.com/doc",
					"title": "Capability Test Doc",
					"content": [{"type": "text", "text": "The secret marker is BANANA-42."}],
					"citations": {"enabled": true}
				},
				{"type": "text", "text": "What is the secret marker?"}
			]
		}]
	}`

	var req AnthropicMessagesRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("expected search_result request to parse, got error: %v", err)
	}

	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	blocks := req.Messages[0].Content.Blocks
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}

	sr := blocks[0]
	if sr.Type != "search_result" {
		t.Errorf("expected first block type search_result, got %q", sr.Type)
	}
	if sr.Source == nil || sr.Source.URL != "https://example.com/doc" {
		t.Errorf("expected string source -> Source.URL=https://example.com/doc, got %+v", sr.Source)
	}
	if sr.Title != "Capability Test Doc" {
		t.Errorf("expected title to be parsed, got %q", sr.Title)
	}
	if got := searchResultText(sr); !strings.Contains(got, "BANANA-42") {
		t.Errorf("expected search_result content text to contain BANANA-42, got %q", got)
	}
}

// TestUnmarshalImageSourceStillObject guards that an object image source
// (the common case) still parses correctly after the string-source addition.
func TestUnmarshalImageSourceStillObject(t *testing.T) {
	body := `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]`
	var content AnthropicContent
	if err := json.Unmarshal([]byte(body), &content); err != nil {
		t.Fatalf("failed to unmarshal image block: %v", err)
	}
	if len(content.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(content.Blocks))
	}
	src := content.Blocks[0].Source
	if src == nil || src.Type != "base64" || src.MediaType != "image/png" || src.Data != "AAAA" {
		t.Errorf("object image source not parsed correctly: %+v", src)
	}
	if src.URL != "" {
		t.Errorf("object image source should not set URL, got %q", src.URL)
	}
}

// TestConvertSearchResultToOpenAI verifies the Chat Completions conversion path
// downgrades a search_result block to plain text.
func TestConvertSearchResultToOpenAI(t *testing.T) {
	blocks := []AnthropicContentBlock{
		{
			Type:   "search_result",
			Source: &AnthropicImageSource{Type: "url", URL: "https://example.com/doc"},
			Title:  "Doc",
			Content: &AnthropicContent{Blocks: []AnthropicContentBlock{
				{Type: "text", Text: "The secret marker is BANANA-42."},
			}},
		},
		{Type: "text", Text: "What is the secret marker?"},
	}

	content, err := convertContentBlocksToOpenAI(blocks)
	if err != nil {
		t.Fatalf("convertContentBlocksToOpenAI error: %v", err)
	}
	if content.Text == nil {
		t.Fatalf("expected combined text output, got parts=%v", content.Parts)
	}
	if !strings.Contains(*content.Text, "BANANA-42") || !strings.Contains(*content.Text, "What is the secret marker?") {
		t.Errorf("expected search_result text + question, got %q", *content.Text)
	}
}

// TestConvertSearchResultWithImageToOpenAI exercises the parts branch (image
// present) so the search_result downgrade also applies there.
func TestConvertSearchResultWithImageToOpenAI(t *testing.T) {
	blocks := []AnthropicContentBlock{
		{
			Type: "search_result",
			Content: &AnthropicContent{Blocks: []AnthropicContentBlock{
				{Type: "text", Text: "BANANA-42"},
			}},
		},
		{Type: "image", Source: &AnthropicImageSource{Type: "base64", MediaType: "image/png", Data: "AAAA"}},
	}

	content, err := convertContentBlocksToOpenAI(blocks)
	if err != nil {
		t.Fatalf("convertContentBlocksToOpenAI error: %v", err)
	}
	var hasText, hasImage bool
	for _, p := range content.Parts {
		if p.Type == "text" && strings.Contains(p.Text, "BANANA-42") {
			hasText = true
		}
		if p.Type == "image_url" {
			hasImage = true
		}
	}
	if !hasText || !hasImage {
		t.Errorf("expected both search_result text and image parts, got %+v", content.Parts)
	}
}

// TestConvertSearchResultToResponses verifies the Responses conversion path
// downgrades a search_result block to input_text.
func TestConvertSearchResultToResponses(t *testing.T) {
	block := AnthropicContentBlock{
		Type: "search_result",
		Content: &AnthropicContent{Blocks: []AnthropicContentBlock{
			{Type: "text", Text: "BANANA-42"},
		}},
	}
	c := convertUserBlockToResponsesContent(block)
	if c == nil {
		t.Fatal("expected non-nil ResponseInputContent for search_result")
	}
	if c.Type != "input_text" || !strings.Contains(c.Text, "BANANA-42") {
		t.Errorf("expected input_text with BANANA-42, got %+v", c)
	}
}
