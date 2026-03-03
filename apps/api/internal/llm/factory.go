package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

func NewProvider(cfg config.LLMConfig) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "local":
		return &localProvider{}, nil
	case "openai":
		if strings.TrimSpace(cfg.OpenAIKey) == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai llm provider")
		}
		return newOpenAIProvider(cfg.OpenAIBaseURL, cfg.OpenAIKey, cfg.OpenAIModel), nil
	case "ollama":
		return newOllamaProvider(cfg.OllamaBaseURL, cfg.OllamaModel), nil
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER: %s", cfg.Provider)
	}
}
