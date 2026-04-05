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

// OpenAIProvider implements [Provider] for any OpenAI-compatible chat
// completions API. This covers OpenAI itself, OpenRouter (which provides a
// unified endpoint for many models), and locally-hosted LLMs that expose the
// standard /chat/completions endpoint (LM Studio, Ollama, llama.cpp, etc.).
// The provider name is always "openai" regardless of the actual endpoint so
// that callers can use the name as a capability label rather than an identity.
type OpenAIProvider struct {
	endpoint string       // base URL without trailing slash
	apiKey   string       // Bearer token value
	model    string       // model string forwarded in the request body
	client   *http.Client
}

// NewOpenAIProvider constructs a ready-to-use [OpenAIProvider].
// Passing an empty endpoint falls back to "https://api.openai.com/v1".
// Passing an empty model falls back to "gpt-4o-mini".
// The trailing slash is stripped from endpoint so path concatenation is safe.
func NewOpenAIProvider(endpoint, apiKey, model string) *OpenAIProvider {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	if model == "" {
		model = "gpt-4o-mini"
	}

	return &OpenAIProvider{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		client:   &http.Client{},
	}
}

// Name returns the provider identifier "openai". OpenRouter and unknown
// providers also report "openai" because they use the same wire protocol.
func (p *OpenAIProvider) Name() string { return "openai" }

// Available reports whether an API key is set. An empty key means every
// Complete call will be rejected by the remote API with a 401, so callers
// can skip the network round-trip entirely.
func (p *OpenAIProvider) Available() bool { return p.apiKey != "" }

// openAIRequest is the JSON body for POST /chat/completions.
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"` // omit zero so API uses its own default
	Temperature float64         `json:"temperature"`
}

// openAIMessage represents a single turn in the OpenAI conversation format.
type openAIMessage struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"` // plain text for this turn
}

// openAIResponse is the wire format returned by POST /chat/completions.
type openAIResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"` // "stop", "length", etc.
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"` // combined input + output token count
	} `json:"usage"`
	Model string `json:"model"` // resolved model identifier echoed by the API
	// Error is present when the API returns an error in the response body.
	// OpenAI can return errors at both the HTTP and JSON level; we handle
	// both by checking this field after a successful HTTP call.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends req to the OpenAI-compatible chat completions endpoint and
// returns the first choice's message content. The system prompt (if set) is
// prepended as a "system" role message per the OpenAI convention. If
// MaxTokens is 0 it defaults to 200 — enough for the short JSON answers
// expected by the matcher while keeping latency and cost low.
//
// OpenRouter-specific courtesy headers (HTTP-Referer, X-Title) are
// appended automatically when the endpoint URL contains "openrouter".
//
// On any transport or API error the returned error wraps [ErrProviderError].
// Callers should treat this as a non-match and fail open.
func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Build the messages slice; system prompt goes first if provided.
	messages := []openAIMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: req.UserPrompt})

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		// Keep responses short; the matcher only needs a small JSON object.
		maxTokens = 200
	}

	body := openAIRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// OpenRouter requires attribution headers for models that surface usage
	// analytics to the upstream provider. These are harmless on other APIs.
	if strings.Contains(p.endpoint, "openrouter") {
		httpReq.Header.Set("HTTP-Referer", "https://one-vial.org")
		httpReq.Header.Set("X-Title", "Vial Secret Vault")
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderError, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// The API can signal an error inside a 200 response body; always check.
	if oaiResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderError, oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("%w: no choices in response", ErrProviderError)
	}

	return &CompletionResponse{
		Content:      oaiResp.Choices[0].Message.Content,
		TokensUsed:   oaiResp.Usage.TotalTokens,
		Model:        oaiResp.Model,
		FinishReason: oaiResp.Choices[0].FinishReason,
	}, nil
}
