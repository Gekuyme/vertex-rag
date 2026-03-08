package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

type Client struct {
	baseURL string
	model   string
	maxK    int
	http    *http.Client
	enabled bool
}

type Document struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type Result struct {
	ID    string
	Score float64
	Rank  int
}

func NewClient(cfg config.RerankerConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	maxK := cfg.MaxResults
	if maxK <= 0 {
		maxK = 30
	}

	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		model:   strings.TrimSpace(cfg.Model),
		maxK:    maxK,
		http:    &http.Client{Timeout: timeout},
		enabled: cfg.Enabled && strings.TrimSpace(cfg.URL) != "",
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) Rerank(ctx context.Context, query string, docs []Document) ([]Result, error) {
	if !c.Enabled() || strings.TrimSpace(query) == "" || len(docs) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"query":     query,
		"model":     c.model,
		"documents": docs,
		"top_k":     min(c.maxK, len(docs)),
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal reranker request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rerank", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create reranker request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute reranker request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("reranker returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Results []struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode reranker response: %w", err)
	}

	results := make([]Result, 0, len(parsed.Results))
	for index, item := range parsed.Results {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		results = append(results, Result{
			ID:    item.ID,
			Score: item.Score,
			Rank:  index + 1,
		})
	}

	return results, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
