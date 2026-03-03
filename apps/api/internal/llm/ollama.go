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

type ollamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type ollamaCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
}

type ollamaCompletionResponse struct {
	Message Message `json:"message"`
}

func newOllamaProvider(baseURL, model string) *ollamaProvider {
	return &ollamaProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *ollamaProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	body, err := json.Marshal(ollamaCompletionRequest{
		Model:       p.model,
		Messages:    request.Messages,
		Stream:      false,
		Temperature: request.Temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal ollama completion request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create ollama completion request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("ollama completion request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return "", fmt.Errorf("ollama completion returned status %d", response.StatusCode)
	}

	var parsed ollamaCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode ollama completion response: %w", err)
	}
	if strings.TrimSpace(parsed.Message.Content) == "" {
		return "", errors.New("ollama completion returned empty content")
	}

	return strings.TrimSpace(parsed.Message.Content), nil
}
