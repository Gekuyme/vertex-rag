package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
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
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	vectors := make([][]float32, len(texts))

	concurrency := 4
	if len(texts) < concurrency {
		concurrency = len(texts)
	}
	sem := make(chan struct{}, concurrency)

	group, groupCtx := errgroup.WithContext(ctx)
	for index, text := range texts {
		index := index
		text := text

		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			requestBody, err := json.Marshal(ollamaEmbeddingRequest{
				Model:  p.model,
				Prompt: text,
			})
			if err != nil {
				return fmt.Errorf("marshal ollama request: %w", err)
			}

			request, err := http.NewRequestWithContext(
				groupCtx,
				http.MethodPost,
				p.baseURL+"/api/embeddings",
				bytes.NewReader(requestBody),
			)
			if err != nil {
				return fmt.Errorf("create ollama request: %w", err)
			}
			request.Header.Set("Content-Type", "application/json")

			response, err := p.httpClient.Do(request)
			if err != nil {
				return fmt.Errorf("ollama request failed: %w", err)
			}
			defer response.Body.Close()

			if response.StatusCode >= 300 {
				return fmt.Errorf("ollama embeddings returned status %d", response.StatusCode)
			}

			var parsedResponse ollamaEmbeddingResponse
			if err := json.NewDecoder(response.Body).Decode(&parsedResponse); err != nil {
				return fmt.Errorf("decode ollama response: %w", err)
			}

			vectors[index] = parsedResponse.Embedding
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return vectors, nil
}
