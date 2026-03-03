package llm

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

type Provider interface {
	Complete(context.Context, CompletionRequest) (string, error)
}
