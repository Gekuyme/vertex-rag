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

type geminiProvider struct {
	baseURL       string
	apiKey        string
	model         string
	modelStrict   string
	modelUnstrict string
	httpClient    *http.Client
	maxRetries    int
	retryBackoff  time.Duration
}

type geminiGenerateContentRequest struct {
	SystemInstruction *geminiContent         `json:"system_instruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type geminiStreamChunk = geminiGenerateContentResponse

func newGeminiProvider(
	baseURL,
	apiKey,
	model,
	modelStrict,
	modelUnstrict string,
	httpTimeout time.Duration,
	maxRetries int,
	retryBackoff time.Duration,
) *geminiProvider {
	if httpTimeout <= 0 {
		httpTimeout = 60 * time.Second
	}
	if retryBackoff <= 0 {
		retryBackoff = 300 * time.Millisecond
	}

	return &geminiProvider{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        strings.TrimSpace(apiKey),
		model:         strings.TrimSpace(model),
		modelStrict:   strings.TrimSpace(modelStrict),
		modelUnstrict: strings.TrimSpace(modelUnstrict),
		httpClient:    &http.Client{Timeout: httpTimeout},
		maxRetries:    maxRetries,
		retryBackoff:  retryBackoff,
	}
}

func (p *geminiProvider) modelForMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict":
		if p.modelStrict != "" {
			return p.modelStrict
		}
	case "unstrict":
		if p.modelUnstrict != "" {
			return p.modelUnstrict
		}
	}
	return p.model
}

func (p *geminiProvider) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = p.modelForMode(request.Mode)
	}
	if model == "" {
		return "", errors.New("gemini model is empty")
	}

	systemInstruction, contents := geminiContentsFromMessages(request.Messages)
	if len(contents) == 0 {
		return "", errors.New("gemini request has no contents")
	}

	body, err := json.Marshal(geminiGenerateContentRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
		GenerationConfig: geminiGenerationConfig{
			Temperature:     request.Temperature,
			MaxOutputTokens: request.MaxTokens,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, model, p.apiKey)
	response, err := retryRequest(ctx, p.maxRetries, p.retryBackoff, func() (*http.Response, error) {
		httpRequest, createErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if createErr != nil {
			return nil, fmt.Errorf("create gemini request: %w", createErr)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		return p.httpClient.Do(httpRequest)
	})
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer response.Body.Close()

	var parsed geminiGenerateContentResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode gemini response: %w", err)
	}
	if response.StatusCode >= 300 {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("gemini returned status %d: %s", response.StatusCode, strings.TrimSpace(parsed.Error.Message))
		}
		return "", fmt.Errorf("gemini returned status %d", response.StatusCode)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", errors.New(strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Candidates) == 0 {
		return "", errors.New("gemini returned no candidates")
	}

	answer := strings.TrimSpace(geminiTextFromContent(parsed.Candidates[0].Content))
	if answer == "" {
		return "", errors.New("gemini returned empty content")
	}
	return answer, nil
}

func (p *geminiProvider) StreamComplete(
	ctx context.Context,
	request CompletionRequest,
	onDelta func(delta string),
) (string, error) {
	if onDelta == nil {
		onDelta = func(string) {}
	}

	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = p.modelForMode(request.Mode)
	}
	if model == "" {
		return "", errors.New("gemini model is empty")
	}

	systemInstruction, contents := geminiContentsFromMessages(request.Messages)
	if len(contents) == 0 {
		return "", errors.New("gemini request has no contents")
	}

	body, err := json.Marshal(geminiGenerateContentRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
		GenerationConfig: geminiGenerationConfig{
			Temperature:     request.Temperature,
			MaxOutputTokens: request.MaxTokens,
		},
	})
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, model, p.apiKey)
	response, err := retryRequest(ctx, p.maxRetries, p.retryBackoff, func() (*http.Response, error) {
		httpRequest, createErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if createErr != nil {
			return nil, fmt.Errorf("create gemini stream request: %w", createErr)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("Accept", "text/event-stream")
		return p.httpClient.Do(httpRequest)
	})
	if err != nil {
		return "", fmt.Errorf("gemini stream request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		var parsed geminiGenerateContentResponse
		if err := json.NewDecoder(response.Body).Decode(&parsed); err == nil && parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("gemini returned status %d: %s", response.StatusCode, strings.TrimSpace(parsed.Error.Message))
		}
		return "", fmt.Errorf("gemini returned status %d", response.StatusCode)
	}

	var builder strings.Builder
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return "", fmt.Errorf("decode gemini stream chunk: %w", err)
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return "", errors.New(strings.TrimSpace(chunk.Error.Message))
		}
		if len(chunk.Candidates) == 0 {
			continue
		}

		delta := geminiRawTextFromContent(chunk.Candidates[0].Content)
		if delta == "" {
			continue
		}

		onDelta(delta)
		builder.WriteString(delta)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read gemini stream: %w", err)
	}

	answer := strings.TrimSpace(builder.String())
	if answer == "" {
		return "", errors.New("gemini stream returned empty content")
	}
	return answer, nil
}

func geminiRawTextFromContent(content geminiContent) string {
	var builder strings.Builder
	for _, part := range content.Parts {
		if part.Text == "" {
			continue
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func geminiContentsFromMessages(messages []Message) (*geminiContent, []geminiContent) {
	var systemParts []geminiPart
	contents := make([]geminiContent, 0, len(messages))

	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "system":
			systemParts = append(systemParts, geminiPart{Text: content})
		case "assistant":
			contents = append(contents, geminiContent{
				Role:  "model",
				Parts: []geminiPart{{Text: content}},
			})
		default:
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: content}},
			})
		}
	}

	if len(systemParts) == 0 {
		return nil, contents
	}
	return &geminiContent{Parts: systemParts}, contents
}

func geminiTextFromContent(content geminiContent) string {
	parts := make([]string, 0, len(content.Parts))
	for _, part := range content.Parts {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
