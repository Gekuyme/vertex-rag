package main

import (
	"context"
	"errors"
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

func main() {
	cfg := config.Load()

	dbStore, err := store.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer dbStore.Close()

	if err := dbStore.Ping(context.Background()); err != nil {
		log.Fatalf("ping db: %v", err)
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
