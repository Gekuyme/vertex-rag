package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	APIAddr     string
	DatabaseURL string
	JWTSecret   string
	AccessTTL   time.Duration
	RefreshTTL  time.Duration
	CORSOrigin  string
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

	cfg := Config{
		APIAddr:     ":" + envOrDefault("API_PORT", "8080"),
		DatabaseURL: envOrDefault("DATABASE_URL", "postgres://vertex:vertex@localhost:5432/vertex_rag?sslmode=disable"),
		JWTSecret:   envOrDefault("JWT_SECRET", "change-me"),
		AccessTTL:   accessTTL,
		RefreshTTL:  refreshTTL,
		CORSOrigin:  envOrDefault("CORS_ORIGIN", "http://localhost:3000"),
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
