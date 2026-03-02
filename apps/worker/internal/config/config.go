package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	WorkerAddr  string
	DatabaseURL string
	Redis       RedisConfig
	S3          S3Config
	Embeddings  EmbeddingConfig
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

type EmbeddingConfig struct {
	Provider      string
	OpenAIKey     string
	OpenAIBaseURL string
	OpenAIModel   string
	OllamaBaseURL string
	OllamaModel   string
}

func Load() Config {
	return Config{
		WorkerAddr:  ":" + envOrDefault("WORKER_PORT", "8082"),
		DatabaseURL: envOrDefault("DATABASE_URL", "postgres://vertex:vertex@localhost:5432/vertex_rag?sslmode=disable"),
		Redis: RedisConfig{
			Addr:     envOrDefault("REDIS_ADDR", "redis:6379"),
			Password: envOrDefault("REDIS_PASSWORD", ""),
			DB:       parseIntWithDefault("REDIS_DB", 0),
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
	}
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
