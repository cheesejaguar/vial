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

// OpenAIProvider handles OpenAI-compatible APIs (OpenAI, OpenRouter, local LLMs).
type OpenAIProvider struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

// NewOpenAIProvider creates a provider for OpenAI-compatible APIs.
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

func (p *OpenAIProvider) Name() string    { return "openai" }
func (p *OpenAIProvider) Available() bool { return p.apiKey != "" }

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := []openAIMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: req.UserPrompt})

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
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

	// OpenRouter-specific header
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
