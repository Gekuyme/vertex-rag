package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const braveSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"

type braveProvider struct {
	apiKey     string
	httpClient *http.Client
}

type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Description   string   `json:"description"`
			ExtraSnippets []string `json:"extra_snippets"`
		} `json:"results"`
	} `json:"web"`
}

func newBraveProvider(apiKey string, timeout time.Duration) *braveProvider {
	if timeout <= 0 {
		timeout = 6 * time.Second
	}

	return &braveProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *braveProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}

	endpoint, err := url.Parse(braveSearchEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse brave endpoint: %w", err)
	}

	queryValues := endpoint.Query()
	queryValues.Set("q", strings.TrimSpace(query))
	queryValues.Set("count", fmt.Sprintf("%d", maxResults))
	endpoint.RawQuery = queryValues.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create brave request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Subscription-Token", p.apiKey)

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("brave request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("brave search returned status %d", response.StatusCode)
	}

	var parsed braveSearchResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}

	results := make([]Result, 0, len(parsed.Web.Results))
	for _, item := range parsed.Web.Results {
		snippet := strings.TrimSpace(item.Description)
		if snippet == "" && len(item.ExtraSnippets) > 0 {
			snippet = strings.TrimSpace(item.ExtraSnippets[0])
		}

		result := Result{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.URL),
			Snippet: snippet,
		}
		if result.Title == "" || result.URL == "" {
			continue
		}
		results = append(results, result)
	}

	return results, nil
}
