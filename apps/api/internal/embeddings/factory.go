package embeddings

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

func NewProvider(cfg config.EmbeddingConfig) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "local":
		return &localProvider{}, nil
	case "openai":
		if strings.TrimSpace(cfg.OpenAIKey) == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai embeddings")
		}
		return newOpenAIProvider(cfg.OpenAIBaseURL, cfg.OpenAIKey, cfg.OpenAIModel), nil
	case "gemini":
		if strings.TrimSpace(cfg.GeminiKey) == "" {
			return nil, errors.New("GEMINI_API_KEY is required for gemini embeddings")
		}
		return newGeminiProvider(cfg.GeminiBaseURL, cfg.GeminiKey, cfg.GeminiModel), nil
	case "ollama":
		return newOllamaProvider(cfg.OllamaBaseURL, cfg.OllamaModel), nil
	default:
		return nil, fmt.Errorf("unsupported EMBED_PROVIDER: %s", cfg.Provider)
	}
}
