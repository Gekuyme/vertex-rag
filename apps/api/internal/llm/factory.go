package llm

import (
	"fmt"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

func NewProvider(cfg config.LLMConfig) (Provider, error) {
	runtime, err := NewRuntime(cfg)
	if err != nil {
		return nil, err
	}

	provider, _, ok := runtime.Resolve(runtime.DefaultProviderID())
	if !ok {
		return nil, fmt.Errorf("default LLM provider is not available: %s", runtime.DefaultProviderID())
	}

	return provider, nil
}
