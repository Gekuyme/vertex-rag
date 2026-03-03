package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/embeddings"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/httpserver"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/ingest"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
	"github.com/hibiken/asynq"
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

func main() {
	cfg := config.Load()

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

	storageClient, err := storage.NewS3Client(context.Background(), cfg.S3)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	embeddingProvider, err := embeddings.NewProvider(cfg.Embeddings)
	if err != nil {
		log.Fatalf("init embeddings provider: %v", err)
	}

	httpSrv := httpserver.New(cfg.WorkerAddr)
	processor := ingest.NewProcessor(dbStore, storageClient, embeddingProvider)

	asynqServer := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"default": 1,
			},
		},
	)

	taskMux := asynq.NewServeMux()
	taskMux.HandleFunc(queue.TypeIngestDocument, processor.HandleDocumentIngest)

	errCh := make(chan error, 2)
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()
	go func() {
		errCh <- asynqServer.Run(taskMux)
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("worker failed: %v", err)
		}
	case sig := <-shutdownSignal:
		log.Printf("worker received signal: %s", sig)
	}

	asynqServer.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("worker graceful shutdown failed: %v", err)
	}
}
