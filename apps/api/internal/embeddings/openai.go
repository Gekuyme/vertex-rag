package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
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

type openAIEmbeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingsResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func newOpenAIProvider(baseURL, apiKey, model string) *openAIProvider {
	return &openAIProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

func (p *openAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	requestBody, err := json.Marshal(openAIEmbeddingsRequest{
		Model: p.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("openai embeddings returned status %d", response.StatusCode)
	}

	var parsedResponse openAIEmbeddingsResponse
	if err := json.NewDecoder(response.Body).Decode(&parsedResponse); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	vectors := make([][]float32, 0, len(parsedResponse.Data))
	for _, item := range parsedResponse.Data {
		vectors = append(vectors, item.Embedding)
	}

	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("openai returned %d embeddings for %d inputs", len(vectors), len(texts))
	}

	return vectors, nil
}
