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

type ollamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

func newOllamaProvider(baseURL, model string) *ollamaProvider {
	return &ollamaProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

func (p *ollamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))

	for _, text := range texts {
		requestBody, err := json.Marshal(ollamaEmbeddingRequest{
			Model:  p.model,
			Prompt: text,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal ollama request: %w", err)
		}

		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			p.baseURL+"/api/embeddings",
			bytes.NewReader(requestBody),
		)
		if err != nil {
			return nil, fmt.Errorf("create ollama request: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := p.httpClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("ollama request failed: %w", err)
		}

		if response.StatusCode >= 300 {
			response.Body.Close()
			return nil, fmt.Errorf("ollama embeddings returned status %d", response.StatusCode)
		}

		var parsedResponse ollamaEmbeddingResponse
		if err := json.NewDecoder(response.Body).Decode(&parsedResponse); err != nil {
			response.Body.Close()
			return nil, fmt.Errorf("decode ollama response: %w", err)
		}
		response.Body.Close()

		vectors = append(vectors, parsedResponse.Embedding)
	}

	return vectors, nil
}
