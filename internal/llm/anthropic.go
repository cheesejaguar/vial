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

// AnthropicProvider implements the Anthropic Messages API.
type AnthropicProvider struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

// NewAnthropicProvider creates a provider for the Anthropic API.
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

func (p *AnthropicProvider) Name() string   { return "anthropic" }
func (p *AnthropicProvider) Available() bool { return p.apiKey != "" }

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
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

	if antResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderError, antResp.Error.Message)
	}

	if len(antResp.Content) == 0 {
		return nil, fmt.Errorf("%w: no content in response", ErrProviderError)
	}

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
