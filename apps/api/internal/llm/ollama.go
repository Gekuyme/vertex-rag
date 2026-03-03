package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ollamaProvider struct {
	baseURL         string
	baseURLStrict   string
	baseURLUnstrict string
	model           string
	modelStrict     string
	modelUnstrict   string
	numCtx          int
	keepAlive       string
	httpClient      *http.Client
	maxRetries      int
	retryBackoff    time.Duration
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

type ollamaStreamChunk struct {
	Message    Message `json:"message"`
	Done       bool    `json:"done"`
	DoneReason string  `json:"done_reason"`
	Error      string  `json:"error"`
}

func newOllamaProvider(
	baseURL string,
	baseURLStrict string,
	baseURLUnstrict string,
	model string,
	modelStrict string,
	modelUnstrict string,
	numCtx int,
	keepAlive string,
	httpTimeout time.Duration,
	maxRetries int,
	retryBackoff time.Duration,
) *ollamaProvider {
	if httpTimeout <= 0 {
		httpTimeout = 180 * time.Second
	}
	if retryBackoff <= 0 {
		retryBackoff = 300 * time.Millisecond
	}

	base := strings.TrimRight(baseURL, "/")
	strict := strings.TrimRight(baseURLStrict, "/")
	unstrict := strings.TrimRight(baseURLUnstrict, "/")
	if strict == "" {
		strict = base
	}
	if unstrict == "" {
		unstrict = base
	}

	return &ollamaProvider{
		baseURL:         base,
		baseURLStrict:   strict,
		baseURLUnstrict: unstrict,
		model:           strings.TrimSpace(model),
		modelStrict:     strings.TrimSpace(modelStrict),
		modelUnstrict:   strings.TrimSpace(modelUnstrict),
		numCtx:          numCtx,
		keepAlive:       strings.TrimSpace(keepAlive),
		httpClient:      &http.Client{Timeout: httpTimeout},
		maxRetries:      maxRetries,
		retryBackoff:    retryBackoff,
	}
}

func (p *ollamaProvider) baseURLForMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict":
		if p.baseURLStrict != "" {
			return p.baseURLStrict
		}
	case "unstrict":
		if p.baseURLUnstrict != "" {
			return p.baseURLUnstrict
		}
	}
	return p.baseURL
}

func (p *ollamaProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
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

	baseURL := p.baseURLForMode(request.Mode)
	if baseURL == "" {
		return "", errors.New("ollama baseURL is empty")
	}

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
		Model:       model,
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

	response, err := retryRequest(ctx, p.maxRetries, p.retryBackoff, func() (*http.Response, error) {
		httpRequest, createErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			baseURL+"/api/chat",
			bytes.NewReader(body),
		)
		if createErr != nil {
			return nil, fmt.Errorf("create ollama completion request: %w", createErr)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		return p.httpClient.Do(httpRequest)
	})
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

func (p *ollamaProvider) StreamComplete(
	ctx context.Context,
	request CompletionRequest,
	onDelta func(delta string),
) (string, error) {
	if onDelta == nil {
		onDelta = func(string) {}
	}

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

	baseURL := p.baseURLForMode(request.Mode)
	if baseURL == "" {
		return "", errors.New("ollama baseURL is empty")
	}

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
		Model:       model,
		Messages:    request.Messages,
		Stream:      true,
		Think:       &think,
		Temperature: request.Temperature,
		Options:     options,
		KeepAlive:   p.keepAlive,
	})
	if err != nil {
		return "", fmt.Errorf("marshal ollama completion request: %w", err)
	}

	response, err := retryRequest(ctx, p.maxRetries, p.retryBackoff, func() (*http.Response, error) {
		httpRequest, createErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			baseURL+"/api/chat",
			bytes.NewReader(body),
		)
		if createErr != nil {
			return nil, fmt.Errorf("create ollama completion request: %w", createErr)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		return p.httpClient.Do(httpRequest)
	})
	if err != nil {
		return "", fmt.Errorf("ollama completion request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return "", fmt.Errorf("ollama completion returned status %d", response.StatusCode)
	}

	reader := bufio.NewReader(response.Body)
	decoder := json.NewDecoder(reader)

	var builder strings.Builder
	for {
		var chunk ollamaStreamChunk
		if err := decoder.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("decode ollama stream chunk: %w", err)
		}
		if strings.TrimSpace(chunk.Error) != "" {
			return "", fmt.Errorf("ollama stream error: %s", strings.TrimSpace(chunk.Error))
		}

		delta := chunk.Message.Content
		if delta != "" {
			onDelta(delta)
			builder.WriteString(delta)
		}

		if chunk.Done {
			break
		}
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", errors.New("ollama completion returned empty content")
	}

	return result, nil
}
