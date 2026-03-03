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
	baseURL       string
	apiKey        string
	model         string
	modelStrict   string
	modelUnstrict string
	httpClient    *http.Client
	maxRetries    int
	retryBackoff  time.Duration
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

func newOpenAIProvider(
	baseURL,
	apiKey,
	model,
	modelStrict,
	modelUnstrict string,
	httpTimeout time.Duration,
	maxRetries int,
	retryBackoff time.Duration,
) *openAIProvider {
	if httpTimeout <= 0 {
		httpTimeout = 60 * time.Second
	}
	if retryBackoff <= 0 {
		retryBackoff = 300 * time.Millisecond
	}

	return &openAIProvider{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		model:         strings.TrimSpace(model),
		modelStrict:   strings.TrimSpace(modelStrict),
		modelUnstrict: strings.TrimSpace(modelUnstrict),
		httpClient:    &http.Client{Timeout: httpTimeout},
		maxRetries:    maxRetries,
		retryBackoff:  retryBackoff,
	}
}

func (p *openAIProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	model := strings.TrimSpace(request.Model)
	if model == "" {
		switch strings.ToLower(strings.TrimSpace(request.Mode)) {
		case "strict":
			model = p.modelStrict
		case "unstrict":
			model = p.modelUnstrict
		}
	}
	if model == "" {
		model = p.model
	}

	body, err := json.Marshal(openAICompletionRequest{
		Model:       model,
		Messages:    request.Messages,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal openai completion request: %w", err)
	}

	response, err := retryRequest(ctx, p.maxRetries, p.retryBackoff, func() (*http.Response, error) {
		httpRequest, createErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			p.baseURL+"/chat/completions",
			bytes.NewReader(body),
		)
		if createErr != nil {
			return nil, fmt.Errorf("create openai completion request: %w", createErr)
		}
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
		httpRequest.Header.Set("Content-Type", "application/json")
		return p.httpClient.Do(httpRequest)
	})
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

func (p *openAIProvider) StreamComplete(
	ctx context.Context,
	request CompletionRequest,
	onDelta func(delta string),
) (string, error) {
	answer, err := p.Complete(ctx, request)
	if err != nil {
		return "", err
	}

	emitChunks(answer, 100, onDelta)
	return answer, nil
}
