package sparsesearch

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
	baseURL   string
	indexName string
	maxHits   int
	http      *http.Client
	enabled   bool
}

type SearchRequest struct {
	OrgID           string
	RoleID          int64
	Query           string
	Variants        []string
	MaxResults      int
	QueryType       string
	RequireMultiDoc bool
}

type SearchHit struct {
	ChunkID    string
	DocumentID string
	Score      float64
	Rank       int
	Query      string
}

func NewClient(cfg config.SparseSearchConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	maxHits := cfg.MaxResults
	if maxHits <= 0 {
		maxHits = 50
	}

	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		indexName: strings.TrimSpace(cfg.IndexName),
		maxHits:   maxHits,
		http:      &http.Client{Timeout: timeout},
		enabled:   strings.EqualFold(cfg.Provider, "opensearch") && strings.TrimSpace(cfg.URL) != "",
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if !c.Enabled() {
		return nil, nil
	}

	queries := uniqueQueries(req.Query, req.Variants)
	if len(queries) == 0 {
		return nil, nil
	}

	allHits := make([]SearchHit, 0)
	for _, query := range queries {
		hits, err := c.searchVariant(ctx, req, query)
		if err != nil {
			return nil, err
		}
		allHits = append(allHits, hits...)
	}

	return allHits, nil
}

func (c *Client) searchVariant(ctx context.Context, req SearchRequest, query string) ([]SearchHit, error) {
	size := req.MaxResults
	if size <= 0 || size > c.maxHits {
		size = c.maxHits
	}

	payload := map[string]any{
		"size": size,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"multi_match": map[string]any{
							"query":  query,
							"type":   "most_fields",
							"fields": []string{"content^4", "section^2", "heading_path^2", "doc_title^3", "chunk_kind"},
						},
					},
				},
				"filter": []any{
					map[string]any{"term": map[string]any{"org_id": req.OrgID}},
					map[string]any{"term": map[string]any{"status": "ready"}},
					map[string]any{"term": map[string]any{"allowed_role_ids": req.RoleID}},
				},
				"should": []any{
					map[string]any{"match_phrase": map[string]any{"content": map[string]any{"query": query, "boost": 2.4}}},
					map[string]any{"match_phrase": map[string]any{"doc_title": map[string]any{"query": query, "boost": 1.8}}},
				},
				"minimum_should_match": 0,
			},
		},
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal opensearch search request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+c.indexName+"/_search", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create opensearch search request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute opensearch search: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("opensearch search returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					ChunkID    string `json:"chunk_id"`
					DocumentID string `json:"document_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode opensearch search response: %w", err)
	}

	hits := make([]SearchHit, 0, len(parsed.Hits.Hits))
	for index, item := range parsed.Hits.Hits {
		chunkID := strings.TrimSpace(item.Source.ChunkID)
		if chunkID == "" {
			chunkID = strings.TrimSpace(item.ID)
		}
		if chunkID == "" {
			continue
		}

		hits = append(hits, SearchHit{
			ChunkID:    chunkID,
			DocumentID: strings.TrimSpace(item.Source.DocumentID),
			Score:      item.Score,
			Rank:       index + 1,
			Query:      query,
		})
	}

	return hits, nil
}

func uniqueQueries(primary string, variants []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(variants)+1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(primary)
	for _, variant := range variants {
		add(variant)
	}

	return out
}
