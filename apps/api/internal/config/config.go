package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv         string
	APIAddr        string
	DatabaseURL    string
	JWTSecret      string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	CORSOrigin     string
	CORSOrigins    []string
	CookieSecure   bool
	CookieSameSite string
	RateLimitRPM   int
	RateLimitBurst int
	Redis          RedisConfig
	Cache          CacheConfig
	S3             S3Config
	Embeddings     EmbeddingConfig
	LLM            LLMConfig
	Search         SearchConfig
	Features       FeatureConfig
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type CacheConfig struct {
	Enabled               bool
	RetrievalTTL          time.Duration
	AnswerTTL             time.Duration
	UnstrictAnswerEnabled bool
}

type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

type SearchConfig struct {
	Enabled    bool
	Provider   string
	APIKey     string
	MaxResults int
	Timeout    time.Duration
}

type FeatureConfig struct {
	UnstrictLegacyToggleWebSearch bool
}

type EmbeddingConfig struct {
	Provider      string
	OpenAIKey     string
	OpenAIBaseURL string
	OpenAIModel   string
	OllamaBaseURL string
	OllamaModel   string
}

type LLMConfig struct {
	Provider              string
	OpenAIKey             string
	OpenAIBaseURL         string
	OpenAIModel           string
	OpenAIModelStrict     string
	OpenAIModelUnstrict   string
	GeminiKey             string
	GeminiBaseURL         string
	GeminiModel           string
	GeminiModelStrict     string
	GeminiModelUnstrict   string
	OllamaBaseURL         string
	OllamaBaseURLStrict   string
	OllamaBaseURLUnstrict string
	OllamaModel           string
	OllamaModelStrict     string
	OllamaModelUnstrict   string
	OllamaNumCtx          int
	OllamaKeepAlive       string
	HTTPTimeout           time.Duration
	MaxRetries            int
	RetryBackoff          time.Duration
	MaxContextChars       int
}

func Load() (Config, error) {
	accessTTL, err := parseDurationWithDefault("JWT_ACCESS_TTL", "15m")
	if err != nil {
		return Config{}, fmt.Errorf("parse JWT_ACCESS_TTL: %w", err)
	}

	refreshTTL, err := parseDurationWithDefault("JWT_REFRESH_TTL", "720h")
	if err != nil {
		return Config{}, fmt.Errorf("parse JWT_REFRESH_TTL: %w", err)
	}
	retrievalTTL, err := parseDurationWithDefault("CACHE_RETRIEVAL_TTL", "10m")
	if err != nil {
		return Config{}, fmt.Errorf("parse CACHE_RETRIEVAL_TTL: %w", err)
	}
	answerTTL, err := parseDurationWithDefault("CACHE_ANSWER_TTL", "10m")
	if err != nil {
		return Config{}, fmt.Errorf("parse CACHE_ANSWER_TTL: %w", err)
	}
	searchTimeout, err := parseDurationWithDefault("SEARCH_HTTP_TIMEOUT", "6s")
	if err != nil {
		return Config{}, fmt.Errorf("parse SEARCH_HTTP_TIMEOUT: %w", err)
	}
	llmHTTPTimeout, err := parseDurationWithDefault("LLM_HTTP_TIMEOUT", "60s")
	if err != nil {
		return Config{}, fmt.Errorf("parse LLM_HTTP_TIMEOUT: %w", err)
	}
	llmRetryBackoff, err := parseDurationWithDefault("LLM_RETRY_BACKOFF", "400ms")
	if err != nil {
		return Config{}, fmt.Errorf("parse LLM_RETRY_BACKOFF: %w", err)
	}

	corsOrigins := parseCSVEnv("CORS_ORIGIN")
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:3000"}
	}
	corsOrigin := corsOrigins[0]

	cfg := Config{
		AppEnv:         strings.ToLower(strings.TrimSpace(envOrDefault("APP_ENV", "development"))),
		APIAddr:        ":" + envOrDefault("API_PORT", "8080"),
		DatabaseURL:    envOrDefault("DATABASE_URL", "postgres://vertex:vertex@localhost:5432/vertex_rag?sslmode=disable"),
		JWTSecret:      envOrDefault("JWT_SECRET", "change-me"),
		AccessTTL:      accessTTL,
		RefreshTTL:     refreshTTL,
		CORSOrigin:     corsOrigin,
		CORSOrigins:    corsOrigins,
		CookieSecure:   parseBoolWithDefault("COOKIE_SECURE", false),
		CookieSameSite: strings.ToLower(strings.TrimSpace(envOrDefault("COOKIE_SAMESITE", "lax"))),
		RateLimitRPM:   parseIntWithDefault("RATE_LIMIT_RPM", 240),
		RateLimitBurst: parseIntWithDefault("RATE_LIMIT_BURST", 60),
		Redis: RedisConfig{
			Addr:     envOrDefault("REDIS_ADDR", "redis:6379"),
			Password: envOrDefault("REDIS_PASSWORD", ""),
			DB:       parseIntWithDefault("REDIS_DB", 0),
		},
		Cache: CacheConfig{
			Enabled:               parseBoolWithDefault("CACHE_ENABLED", true),
			RetrievalTTL:          retrievalTTL,
			AnswerTTL:             answerTTL,
			UnstrictAnswerEnabled: parseBoolWithDefault("CACHE_UNSTRICT_ANSWER", false),
		},
		S3: S3Config{
			Endpoint:  envOrDefault("S3_ENDPOINT", "minio:9000"),
			Region:    envOrDefault("S3_REGION", "us-east-1"),
			Bucket:    envOrDefault("S3_BUCKET", "vertex-rag"),
			AccessKey: envOrDefault("S3_ACCESS_KEY", "minioadmin"),
			SecretKey: envOrDefault("S3_SECRET_KEY", "minioadmin"),
			UseSSL:    parseBoolWithDefault("S3_USE_SSL", false),
		},
		Embeddings: EmbeddingConfig{
			Provider:      envOrDefault("EMBED_PROVIDER", "local"),
			OpenAIKey:     envOrDefault("OPENAI_API_KEY", ""),
			OpenAIBaseURL: envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIModel:   envOrDefault("EMBED_MODEL_OPENAI", "text-embedding-3-small"),
			OllamaBaseURL: envOrDefault("OLLAMA_BASE_URL", "http://ollama:11434"),
			OllamaModel:   envOrDefault("EMBED_MODEL_OLLAMA", "nomic-embed-text"),
		},
		LLM: LLMConfig{
			Provider:            envOrDefault("LLM_PROVIDER", "local"),
			OpenAIKey:           envOrDefault("OPENAI_API_KEY", ""),
			OpenAIBaseURL:       envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIModel:         envOrDefault("LLM_MODEL_OPENAI", "gpt-4o-mini"),
			OpenAIModelStrict:   envOrDefault("LLM_MODEL_OPENAI_STRICT", ""),
			OpenAIModelUnstrict: envOrDefault("LLM_MODEL_OPENAI_UNSTRICT", ""),
			GeminiKey:           envOrDefault("GEMINI_API_KEY", ""),
			GeminiBaseURL:       envOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
			GeminiModel:         envOrDefault("LLM_MODEL_GEMINI", "gemini-2.5-flash-lite"),
			GeminiModelStrict:   envOrDefault("LLM_MODEL_GEMINI_STRICT", ""),
			GeminiModelUnstrict: envOrDefault("LLM_MODEL_GEMINI_UNSTRICT", ""),
			// Keep backwards compatibility: OLLAMA_BASE_URL still works for both embeddings and LLM.
			// LLM-specific overrides allow pointing strict/unstrict to different Ollama hosts.
			OllamaBaseURL:         envOrDefault("LLM_OLLAMA_BASE_URL", envOrDefault("OLLAMA_BASE_URL", "http://ollama:11434")),
			OllamaBaseURLStrict:   envOrDefault("LLM_OLLAMA_BASE_URL_STRICT", envOrDefault("LLM_OLLAMA_BASE_URL", envOrDefault("OLLAMA_BASE_URL", "http://ollama:11434"))),
			OllamaBaseURLUnstrict: envOrDefault("LLM_OLLAMA_BASE_URL_UNSTRICT", envOrDefault("LLM_OLLAMA_BASE_URL", envOrDefault("OLLAMA_BASE_URL", "http://ollama:11434"))),
			OllamaModel:           envOrDefault("LLM_MODEL_OLLAMA", "llama3.2"),
			OllamaModelStrict:     envOrDefault("LLM_MODEL_OLLAMA_STRICT", ""),
			OllamaModelUnstrict:   envOrDefault("LLM_MODEL_OLLAMA_UNSTRICT", ""),
			OllamaNumCtx:          parseIntWithDefault("LLM_OLLAMA_NUM_CTX", 4096),
			OllamaKeepAlive:       envOrDefault("LLM_OLLAMA_KEEP_ALIVE", ""),
			HTTPTimeout:           llmHTTPTimeout,
			MaxRetries:            parseIntWithDefault("LLM_MAX_RETRIES", 2),
			RetryBackoff:          llmRetryBackoff,
			MaxContextChars:       parseIntWithDefault("LLM_MAX_CONTEXT_CHARS", 7000),
		},
		Search: SearchConfig{
			Enabled:    parseBoolWithDefault("WEB_SEARCH_ENABLED", false),
			Provider:   envOrDefault("SEARCH_API_PROVIDER", "brave"),
			APIKey:     envOrDefault("SEARCH_API_KEY", ""),
			MaxResults: parseIntWithDefault("SEARCH_MAX_RESULTS", 5),
			Timeout:    searchTimeout,
		},
		Features: FeatureConfig{
			UnstrictLegacyToggleWebSearch: parseBoolWithDefault("UNSTRICT_LEGACY_TOGGLE_WEB_SEARCH", true),
		},
	}
	if cfg.RateLimitRPM < 0 {
		cfg.RateLimitRPM = 0
	}
	if cfg.RateLimitBurst < 1 {
		cfg.RateLimitBurst = 1
	}
	if cfg.LLM.MaxRetries < 0 {
		cfg.LLM.MaxRetries = 0
	}
	if cfg.LLM.MaxContextChars < 500 {
		cfg.LLM.MaxContextChars = 7000
	}
	if cfg.AppEnv == "production" && strings.TrimSpace(cfg.JWTSecret) == "change-me" {
		return Config{}, fmt.Errorf("JWT_SECRET must be changed in production")
	}
	if cfg.CookieSameSite != "lax" && cfg.CookieSameSite != "strict" && cfg.CookieSameSite != "none" {
		cfg.CookieSameSite = "lax"
	}
	if cfg.CookieSameSite == "none" {
		cfg.CookieSecure = true
	}

	return cfg, nil
}

func parseDurationWithDefault(key, fallback string) (time.Duration, error) {
	value := envOrDefault(key, fallback)

	return time.ParseDuration(value)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func parseBoolWithDefault(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseIntWithDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsedValue
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	values := strings.Split(raw, ",")
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return normalized
}
