package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/cache"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/embeddings"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/httpserver"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/llm"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/websearch"
)

func waitForDB(ctx context.Context, dbStore *store.Store) error {
	deadline := time.Now().Add(60 * time.Second)
	backoff := 200 * time.Millisecond

	var lastErr error
	for time.Now().Before(deadline) {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := dbStore.Ping(attemptCtx)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		log.Printf("db not ready yet: %v", err)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}

		backoff *= 2
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
	}

	return fmt.Errorf("db not ready after 60s: %w", lastErr)
}

func parseSameSiteMode(raw string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbStore, err := store.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer dbStore.Close()

	startupCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := waitForDB(startupCtx, dbStore); err != nil {
		log.Fatalf("wait for db: %v", err)
	}

	tokenManager, err := auth.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	if err != nil {
		log.Fatalf("init auth manager: %v", err)
	}

	storageClient, err := storage.NewS3Client(context.Background(), cfg.S3)
	if err != nil {
		log.Fatalf("init storage client: %v", err)
	}

	queueClient := queue.NewClient(cfg.Redis)
	defer func() {
		if err := queueClient.Close(); err != nil {
			log.Printf("close queue client: %v", err)
		}
	}()
	cacheClient := cache.NewClient(cfg.Redis, cfg.Cache)
	defer func() {
		if err := cacheClient.Close(); err != nil {
			log.Printf("close cache client: %v", err)
		}
	}()

	embeddingProvider, err := embeddings.NewProvider(cfg.Embeddings)
	if err != nil {
		log.Fatalf("init embeddings provider: %v", err)
	}
	llmProvider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		log.Fatalf("init llm provider: %v", err)
	}
	searchClient, err := websearch.NewClient(cfg.Search)
	if err != nil {
		log.Fatalf("init web search client: %v", err)
	}

	server := httpserver.New(
		cfg.APIAddr,
		dbStore,
		tokenManager,
		storageClient,
		queueClient,
		cacheClient,
		embeddingProvider,
		llmProvider,
		searchClient,
		cfg.CORSOrigins,
		cfg.CookieSecure,
		parseSameSiteMode(cfg.CookieSameSite),
		cfg.RateLimitRPM,
		cfg.RateLimitBurst,
		cfg.LLM.MaxContextChars,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("api server failed: %v", err)
		}
	case sig := <-shutdownSignal:
		log.Printf("api received signal: %s", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("api graceful shutdown failed: %v", err)
	}

	log.Println("api server shut down")
}
