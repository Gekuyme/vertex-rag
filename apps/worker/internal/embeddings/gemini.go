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

type geminiProvider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type geminiBatchEmbedRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiEmbedRequest struct {
	Model   string             `json:"model"`
	Content geminiEmbedContent `json:"content"`
}

type geminiEmbedContent struct {
	Parts []geminiEmbedPart `json:"parts"`
}

type geminiEmbedPart struct {
	Text string `json:"text"`
}

type geminiBatchEmbedResponse struct {
	Embeddings []struct {
		Values []float32 `json:"values"`
	} `json:"embeddings"`
}

func newGeminiProvider(baseURL, apiKey, model string) *geminiProvider {
	return &geminiProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     strings.TrimSpace(apiKey),
		model:      strings.TrimSpace(model),
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

func (p *geminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	requests := make([]geminiEmbedRequest, 0, len(texts))
	for _, text := range texts {
		requests = append(requests, geminiEmbedRequest{
			Model: "models/" + p.model,
			Content: geminiEmbedContent{
				Parts: []geminiEmbedPart{{Text: text}},
			},
		})
	}

	requestBody, err := json.Marshal(geminiBatchEmbedRequest{Requests: requests})
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/models/"+p.model+":batchEmbedContents",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create gemini request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", p.apiKey)

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini embeddings returned status %d", response.StatusCode)
	}

	var parsedResponse geminiBatchEmbedResponse
	if err := json.NewDecoder(response.Body).Decode(&parsedResponse); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}

	vectors := make([][]float32, 0, len(parsedResponse.Embeddings))
	for _, item := range parsedResponse.Embeddings {
		vectors = append(vectors, item.Values)
	}

	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("gemini returned %d embeddings for %d inputs", len(vectors), len(texts))
	}

	return vectors, nil
}
