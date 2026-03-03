package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type openAIProvider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type openAICompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type openAICompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func newOpenAIProvider(baseURL, apiKey, model string) *openAIProvider {
	return &openAIProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *openAIProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	body, err := json.Marshal(openAICompletionRequest{
		Model:       p.model,
		Messages:    request.Messages,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal openai completion request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create openai completion request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("openai completion request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return "", fmt.Errorf("openai completion returned status %d", response.StatusCode)
	}

	var parsed openAICompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode openai completion response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("openai completion returned no choices")
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
