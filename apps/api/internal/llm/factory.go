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
		return newOpenAIProvider(
			cfg.OpenAIBaseURL,
			cfg.OpenAIKey,
			cfg.OpenAIModel,
			cfg.OpenAIModelStrict,
			cfg.OpenAIModelUnstrict,
			cfg.HTTPTimeout,
			cfg.MaxRetries,
			cfg.RetryBackoff,
		), nil
	case "gemini":
		if strings.TrimSpace(cfg.GeminiKey) == "" {
			return nil, errors.New("GEMINI_API_KEY is required for gemini llm provider")
		}
		return newGeminiProvider(
			cfg.GeminiBaseURL,
			cfg.GeminiKey,
			cfg.GeminiModel,
			cfg.GeminiModelStrict,
			cfg.GeminiModelUnstrict,
			cfg.HTTPTimeout,
			cfg.MaxRetries,
			cfg.RetryBackoff,
		), nil
	case "ollama":
		return newOllamaProvider(
			cfg.OllamaBaseURL,
			cfg.OllamaBaseURLStrict,
			cfg.OllamaBaseURLUnstrict,
			cfg.OllamaModel,
			cfg.OllamaModelStrict,
			cfg.OllamaModelUnstrict,
			cfg.OllamaNumCtx,
			cfg.OllamaKeepAlive,
			cfg.HTTPTimeout,
			cfg.MaxRetries,
			cfg.RetryBackoff,
		), nil
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER: %s", cfg.Provider)
	}
}
