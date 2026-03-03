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
	numCtx     int
	keepAlive  string
	httpClient *http.Client
}

type ollamaCompletionRequest struct {
	Model       string         `json:"model"`
	Messages    []Message      `json:"messages"`
	Stream      bool           `json:"stream"`
	Think       *bool          `json:"think,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	KeepAlive   string         `json:"keep_alive,omitempty"`
}

type ollamaCompletionResponse struct {
	Message Message `json:"message"`
}

func newOllamaProvider(baseURL, model string, numCtx int, keepAlive string) *ollamaProvider {
	return &ollamaProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		numCtx:     numCtx,
		keepAlive:  strings.TrimSpace(keepAlive),
		httpClient: &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *ollamaProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	think := false
	options := map[string]any{}
	if request.MaxTokens > 0 {
		options["num_predict"] = request.MaxTokens
	}
	if p.numCtx > 0 {
		options["num_ctx"] = p.numCtx
	}
	if len(options) == 0 {
		options = nil
	}

	body, err := json.Marshal(ollamaCompletionRequest{
		Model:       p.model,
		Messages:    request.Messages,
		Stream:      false,
		Think:       &think,
		Temperature: request.Temperature,
		Options:     options,
		KeepAlive:   p.keepAlive,
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
