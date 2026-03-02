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

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/httpserver"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

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

	if err := dbStore.Ping(context.Background()); err != nil {
		log.Fatalf("ping db: %v", err)
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

	server := httpserver.New(cfg.APIAddr, dbStore, tokenManager, storageClient, queueClient, cfg.CORSOrigin)

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
