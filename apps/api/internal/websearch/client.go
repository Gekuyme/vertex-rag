package websearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

type Result struct {
	Title   string
	URL     string
	Snippet string
}

type Provider interface {
	Search(context.Context, string, int) ([]Result, error)
}

type Client struct {
	enabled    bool
	maxResults int
	provider   Provider
}

func NewClient(cfg config.SearchConfig) (*Client, error) {
	client := &Client{
		enabled:    cfg.Enabled,
		maxResults: cfg.MaxResults,
	}
	if client.maxResults <= 0 {
		client.maxResults = 5
	}
	if !client.enabled {
		return client, nil
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "brave":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil, fmt.Errorf("SEARCH_API_KEY is required when WEB_SEARCH_ENABLED=true")
		}
		client.provider = newBraveProvider(cfg.APIKey, cfg.Timeout)
	default:
		return nil, fmt.Errorf("unsupported SEARCH_API_PROVIDER: %s", cfg.Provider)
	}

	return client, nil
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled && c.provider != nil
}

func (c *Client) Search(ctx context.Context, query string) ([]Result, error) {
	if !c.Enabled() {
		return []Result{}, nil
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []Result{}, nil
	}

	return c.provider.Search(ctx, trimmedQuery, c.maxResults)
}
