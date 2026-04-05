package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicProvider implements [Provider] using the Anthropic Messages API
// (POST /v1/messages). It maps Vial's generic [CompletionRequest] onto the
// Anthropic request shape, which separates the system prompt from the
// message list.
type AnthropicProvider struct {
	endpoint string      // base URL, trailing slash stripped
	apiKey   string      // Anthropic API key (x-api-key header)
	model    string      // model string forwarded to the API
	client   *http.Client
}

// NewAnthropicProvider constructs a ready-to-use [AnthropicProvider].
// Passing an empty endpoint falls back to "https://api.anthropic.com".
// Passing an empty model falls back to "claude-sonnet-4-6".
// The trailing slash is stripped from endpoint so path concatenation is safe.
func NewAnthropicProvider(endpoint, apiKey, model string) *AnthropicProvider {
	if endpoint == "" {
		endpoint = "https://api.anthropic.com"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	if model == "" {
		model = "claude-sonnet-4-6"
	}

	return &AnthropicProvider{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		client:   &http.Client{},
	}
}

// Name returns the provider identifier "anthropic".
func (p *AnthropicProvider) Name() string { return "anthropic" }

// Available reports whether an API key is set. An empty key means every
// Complete call will be rejected by the remote API, so callers can skip the
// network round-trip entirely.
func (p *AnthropicProvider) Available() bool { return p.apiKey != "" }

// anthropicRequest is the wire format for POST /v1/messages.
// System is a top-level field on the Anthropic schema, not a message role.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"` // optional instruction context
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicMessage represents a single turn in the Anthropic conversation format.
type anthropicMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // plain text for this turn
}

// anthropicResponse is the wire format returned by POST /v1/messages.
// Content is a slice to allow multi-block responses (text, tool_use, etc.),
// but Vial only uses text blocks.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"` // e.g. "text", "tool_use"
		Text string `json:"text"` // present when Type == "text"
	} `json:"content"`
	StopReason string `json:"stop_reason"` // e.g. "end_turn", "max_tokens"
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"` // resolved model identifier echoed by the API
	// Error is populated when the HTTP response is a non-2xx status that the
	// Anthropic API encodes in the body rather than via HTTP status codes.
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends req to the Anthropic Messages API and returns the first
// text block from the response. If MaxTokens is 0 it defaults to 200 to
// avoid unbounded responses for the short JSON answers expected by the
// matcher. The returned error wraps [ErrProviderError] for transport or
// API-level failures; callers should treat any error as a non-match and
// fail open.
func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		// 200 tokens is more than enough for the matcher's JSON response;
		// keeping it low reduces latency and cost.
		maxTokens = 200
	}

	body := anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		System:    req.SystemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: req.UserPrompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	// anthropic-version is required by the API; pin to a stable version to
	// avoid unexpected schema changes when Anthropic releases new revisions.
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderError, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var antResp anthropicResponse
	if err := json.Unmarshal(respBody, &antResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// The Anthropic API can return an error object inside a 200 response body,
	// so we must inspect the body rather than relying solely on HTTP status.
	if antResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderError, antResp.Error.Message)
	}

	if len(antResp.Content) == 0 {
		return nil, fmt.Errorf("%w: no content in response", ErrProviderError)
	}

	// Concatenate all text blocks; non-text blocks (tool_use, etc.) are
	// silently ignored because the matcher only needs plain text JSON.
	var content string
	for _, c := range antResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &CompletionResponse{
		Content:      content,
		TokensUsed:   antResp.Usage.InputTokens + antResp.Usage.OutputTokens,
		Model:        antResp.Model,
		FinishReason: antResp.StopReason,
	}, nil
}
