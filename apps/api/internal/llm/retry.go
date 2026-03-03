package llm

import (
	"context"
	"io"
	"net/http"
	"time"
)

func shouldRetryStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func retryRequest(
	ctx context.Context,
	maxRetries int,
	baseBackoff time.Duration,
	request func() (*http.Response, error),
) (*http.Response, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if baseBackoff <= 0 {
		baseBackoff = 300 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		response, err := request()
		if err == nil && response != nil && !shouldRetryStatus(response.StatusCode) {
			return response, nil
		}
		if err != nil {
			lastErr = err
		}

		if attempt == maxRetries {
			if response != nil && shouldRetryStatus(response.StatusCode) {
				return response, nil
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return response, nil
		}
		if response != nil && shouldRetryStatus(response.StatusCode) {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}

		backoff := baseBackoff * time.Duration(1<<attempt)
		if backoff > 4*time.Second {
			backoff = 4 * time.Second
		}

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}
