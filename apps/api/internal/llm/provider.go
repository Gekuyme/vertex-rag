package llm

import (
	"context"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	// Mode is a hint for providers that want to use different models/settings
	// for strict vs unstrict runs.
	Mode     string
	Messages []Message
	// Model optionally overrides the provider's default model.
	Model       string
	MaxTokens   int
	Temperature float64
}

type Provider interface {
	Complete(context.Context, CompletionRequest) (string, error)
	StreamComplete(context.Context, CompletionRequest, func(delta string)) (string, error)
}

// StreamProvider is an optional interface for providers that can stream deltas.
// Implementations should call onDelta with incremental text chunks in order.
type StreamProvider interface {
	StreamComplete(context.Context, CompletionRequest, func(delta string)) (string, error)
}

func emitChunks(answer string, chunkSize int, onDelta func(delta string)) {
	if onDelta == nil {
		return
	}
	for _, chunk := range splitChunks(answer, chunkSize) {
		onDelta(chunk)
	}
}

func splitChunks(value string, chunkSize int) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return []string{}
	}
	if chunkSize <= 0 {
		chunkSize = 80
	}

	runes := []rune(trimmed)
	chunks := make([]string, 0, len(runes)/chunkSize+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}

	return chunks
}
