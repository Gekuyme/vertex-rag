package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
)

type ProviderOption struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	DefaultModel string   `json:"default_model"`
	Models       []string `json:"models"`
}

type Runtime struct {
	defaultProviderID string
	providers         map[string]Provider
	options           map[string]ProviderOption
	orderedOptions    []ProviderOption
}

func NewRuntime(cfg config.LLMConfig) (*Runtime, error) {
	runtime := &Runtime{
		defaultProviderID: strings.ToLower(strings.TrimSpace(cfg.Provider)),
		providers:         map[string]Provider{},
		options:           map[string]ProviderOption{},
		orderedOptions:    make([]ProviderOption, 0, 4),
	}

	runtime.addProvider("local", "Local", []string{}, "", &localProvider{})

	if strings.TrimSpace(cfg.OpenAIKey) != "" {
		openAIModels := uniqueNonEmpty(cfg.OpenAIModel, cfg.OpenAIModelStrict, cfg.OpenAIModelUnstrict)
		runtime.addProvider(
			"openai",
			"OpenAI",
			openAIModels,
			firstNonEmpty(cfg.OpenAIModel, cfg.OpenAIModelStrict, cfg.OpenAIModelUnstrict),
			newOpenAIProvider(
				cfg.OpenAIBaseURL,
				cfg.OpenAIKey,
				cfg.OpenAIModel,
				cfg.OpenAIModelStrict,
				cfg.OpenAIModelUnstrict,
				cfg.HTTPTimeout,
				cfg.MaxRetries,
				cfg.RetryBackoff,
			),
		)
	}

	if strings.TrimSpace(cfg.GeminiKey) != "" {
		geminiModels := uniqueNonEmpty(cfg.GeminiModel, cfg.GeminiModelStrict, cfg.GeminiModelUnstrict)
		runtime.addProvider(
			"gemini",
			"Gemini",
			geminiModels,
			firstNonEmpty(cfg.GeminiModel, cfg.GeminiModelStrict, cfg.GeminiModelUnstrict),
			newGeminiProvider(
				cfg.GeminiBaseURL,
				cfg.GeminiKey,
				cfg.GeminiModel,
				cfg.GeminiModelStrict,
				cfg.GeminiModelUnstrict,
				cfg.HTTPTimeout,
				cfg.MaxRetries,
				cfg.RetryBackoff,
			),
		)
	}

	if strings.TrimSpace(cfg.OllamaBaseURL) != "" || strings.TrimSpace(cfg.OllamaBaseURLStrict) != "" || strings.TrimSpace(cfg.OllamaBaseURLUnstrict) != "" {
		ollamaModels := uniqueNonEmpty(cfg.OllamaModel, cfg.OllamaModelStrict, cfg.OllamaModelUnstrict)
		runtime.addProvider(
			"ollama",
			"Ollama",
			ollamaModels,
			firstNonEmpty(cfg.OllamaModel, cfg.OllamaModelStrict, cfg.OllamaModelUnstrict),
			newOllamaProvider(
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
			),
		)
	}

	if runtime.defaultProviderID == "" {
		runtime.defaultProviderID = "local"
	}
	if _, ok := runtime.providers[runtime.defaultProviderID]; !ok {
		return nil, fmt.Errorf("default LLM provider is not available: %s", cfg.Provider)
	}

	return runtime, nil
}

func (r *Runtime) DefaultProviderID() string {
	if r == nil || strings.TrimSpace(r.defaultProviderID) == "" {
		return "local"
	}
	return r.defaultProviderID
}

func (r *Runtime) Options() []ProviderOption {
	if r == nil {
		return nil
	}
	options := make([]ProviderOption, len(r.orderedOptions))
	copy(options, r.orderedOptions)
	return options
}

func (r *Runtime) Resolve(providerID string) (Provider, ProviderOption, bool) {
	if r == nil {
		return nil, ProviderOption{}, false
	}
	resolvedID := strings.ToLower(strings.TrimSpace(providerID))
	if resolvedID == "" {
		resolvedID = r.DefaultProviderID()
	}
	provider, ok := r.providers[resolvedID]
	if !ok {
		return nil, ProviderOption{}, false
	}
	option := r.options[resolvedID]
	return provider, option, true
}

func (r *Runtime) ValidateModel(providerID, model string) error {
	_, option, ok := r.Resolve(providerID)
	if !ok {
		return errors.New("unsupported llm provider")
	}
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return nil
	}
	if len(option.Models) == 0 {
		return errors.New("this llm provider does not support model overrides")
	}
	for _, candidate := range option.Models {
		if candidate == trimmedModel {
			return nil
		}
	}
	return fmt.Errorf("unsupported llm model for provider %s", option.ID)
}

func (r *Runtime) addProvider(id, label string, models []string, defaultModel string, provider Provider) {
	normalizedID := strings.ToLower(strings.TrimSpace(id))
	if normalizedID == "" || provider == nil {
		return
	}
	option := ProviderOption{
		ID:           normalizedID,
		Label:        strings.TrimSpace(label),
		DefaultModel: strings.TrimSpace(defaultModel),
		Models:       uniqueNonEmpty(models...),
	}
	if option.Label == "" {
		option.Label = normalizedID
	}
	r.providers[normalizedID] = provider
	r.options[normalizedID] = option
	r.orderedOptions = append(r.orderedOptions, option)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
