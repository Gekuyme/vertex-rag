package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/embeddings"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
	"github.com/hibiken/asynq"
)

type Processor struct {
	store      *store.Store
	storage    *storage.Client
	embeddings embeddings.Provider
}

func NewProcessor(dbStore *store.Store, storageClient *storage.Client, embeddingsProvider embeddings.Provider) *Processor {
	return &Processor{
		store:      dbStore,
		storage:    storageClient,
		embeddings: embeddingsProvider,
	}
}

func (p *Processor) HandleDocumentIngest(ctx context.Context, task *asynq.Task) error {
	var payload queue.IngestDocumentPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", asynq.SkipRetry)
	}
	if payload.DocumentID == "" {
		return fmt.Errorf("document_id is required: %w", asynq.SkipRetry)
	}

	document, err := p.store.GetDocumentForIngestion(ctx, payload.DocumentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("document not found: %w", asynq.SkipRetry)
		}
		return fmt.Errorf("load document: %w", err)
	}

	if err := p.store.UpdateDocumentStatus(ctx, document.ID, "processing"); err != nil {
		return fmt.Errorf("set status processing: %w", err)
	}

	if err := p.ingestDocument(ctx, document); err != nil {
		_ = p.store.UpdateDocumentStatus(ctx, document.ID, "failed")
		return err
	}

	if err := p.store.UpdateDocumentStatus(ctx, document.ID, "ready"); err != nil {
		return fmt.Errorf("set status ready: %w", err)
	}
	if err := p.store.IncrementOrganizationKBVersion(ctx, document.OrgID); err != nil {
		return fmt.Errorf("increment kb version: %w", err)
	}

	log.Printf("ingest completed for document=%s", document.ID)
	return nil
}

func (p *Processor) ingestDocument(ctx context.Context, document store.DocumentForIngestion) error {
	reader, err := p.storage.Download(ctx, document.StorageKey)
	if err != nil {
		return fmt.Errorf("download document: %w", err)
	}
	defer reader.Close()

	text, err := extractText(ctx, document.MIME, document.Filename, reader)
	if err != nil {
		return fmt.Errorf("extract text: %w", err)
	}

	chunks := chunkText(text)
	if len(chunks) == 0 {
		return errors.New("no chunks generated")
	}

	chunkTexts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunkTexts = append(chunkTexts, chunk.Content)
	}

	vectors, err := p.embeddings.Embed(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, expected %d", len(vectors), len(chunks))
	}

	for index := range chunks {
		chunks[index].Embedding = vectors[index]
	}

	if err := p.store.ReplaceDocumentChunks(ctx, document, chunks); err != nil {
		return fmt.Errorf("replace chunks: %w", err)
	}

	return nil
}
