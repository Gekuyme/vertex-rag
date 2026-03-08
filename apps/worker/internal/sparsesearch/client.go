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

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
)

type Client struct {
	baseURL   string
	indexName string
	http      *http.Client
	enabled   bool
}

type IndexedChunk struct {
	ChunkID        string         `json:"chunk_id"`
	OrgID          string         `json:"org_id"`
	DocumentID     string         `json:"document_id"`
	DocTitle       string         `json:"doc_title"`
	DocFilename    string         `json:"doc_filename"`
	ChunkIndex     int            `json:"chunk_index"`
	Content        string         `json:"content"`
	Section        string         `json:"section"`
	HeadingPath    string         `json:"heading_path"`
	ChunkKind      string         `json:"chunk_kind"`
	AllowedRoleIDs []int64        `json:"allowed_role_ids"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata"`
}

func NewClient(cfg config.SparseSearchConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		indexName: strings.TrimSpace(cfg.IndexName),
		http:      &http.Client{Timeout: timeout},
		enabled:   cfg.Enabled && strings.TrimSpace(cfg.URL) != "" && strings.EqualFold(cfg.Provider, "opensearch"),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) EnsureIndex(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}

	mapping := map[string]any{
		"settings": map[string]any{
			"index": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"chunk_id":         map[string]any{"type": "keyword"},
				"org_id":           map[string]any{"type": "keyword"},
				"document_id":      map[string]any{"type": "keyword"},
				"doc_title":        map[string]any{"type": "text"},
				"doc_filename":     map[string]any{"type": "keyword"},
				"chunk_index":      map[string]any{"type": "integer"},
				"content":          map[string]any{"type": "text"},
				"section":          map[string]any{"type": "text"},
				"heading_path":     map[string]any{"type": "text"},
				"chunk_kind":       map[string]any{"type": "keyword"},
				"allowed_role_ids": map[string]any{"type": "long"},
				"status":           map[string]any{"type": "keyword"},
				"metadata":         map[string]any{"type": "object", "enabled": true},
			},
		},
	}

	requestBody, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal opensearch mapping: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/"+c.indexName, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create ensure index request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("ensure opensearch index: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
		return nil
	}
	if response.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		if strings.Contains(string(body), "resource_already_exists_exception") {
			return nil
		}
	}

	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return fmt.Errorf("ensure opensearch index returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}

func (c *Client) ReplaceDocument(ctx context.Context, document store.DocumentForIngestion, chunks []IndexedChunk) error {
	if !c.Enabled() {
		return nil
	}
	if err := c.EnsureIndex(ctx); err != nil {
		return err
	}

	if err := c.DeleteDocument(ctx, document.ID); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, chunk := range chunks {
		action := map[string]any{
			"index": map[string]any{
				"_index": c.indexName,
				"_id":    chunk.ChunkID,
			},
		}
		if err := encoder.Encode(action); err != nil {
			return fmt.Errorf("encode bulk action: %w", err)
		}
		if err := encoder.Encode(chunk); err != nil {
			return fmt.Errorf("encode bulk document: %w", err)
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/_bulk", &body)
	if err != nil {
		return fmt.Errorf("create bulk request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-ndjson")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("bulk index chunks: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("bulk index returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (c *Client) DeleteDocument(ctx context.Context, documentID string) error {
	if !c.Enabled() || strings.TrimSpace(documentID) == "" {
		return nil
	}

	payload := map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				"document_id": documentID,
			},
		},
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal delete query: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+c.indexName+"/_delete_by_query", bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create delete query request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("delete document chunks from opensearch: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return fmt.Errorf("delete document returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}
