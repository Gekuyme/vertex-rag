package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/embeddings"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/sparsesearch"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/worker/internal/store"
	"github.com/hibiken/asynq"
)

type Processor struct {
	store      *store.Store
	storage    *storage.Client
	embeddings embeddings.Provider
	sparse     *sparsesearch.Client
}

func NewProcessor(dbStore *store.Store, storageClient *storage.Client, embeddingsProvider embeddings.Provider, sparseClient *sparsesearch.Client) *Processor {
	return &Processor{
		store:      dbStore,
		storage:    storageClient,
		embeddings: embeddingsProvider,
		sparse:     sparseClient,
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

	plan := chunkDocumentText(text, document.MIME, document.Filename)
	if len(plan.Chunks) == 0 {
		return errors.New("no chunks generated")
	}

	chunkTexts := make([]string, 0, len(plan.Chunks))
	for _, chunk := range plan.Chunks {
		chunkTexts = append(chunkTexts, buildEmbeddingInput(document.Title, document.Filename, chunk))
	}

	vectors, err := p.embeddings.Embed(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}
	if len(vectors) != len(plan.Chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, expected %d", len(vectors), len(plan.Chunks))
	}

	for index := range plan.Chunks {
		plan.Chunks[index].Embedding = vectors[index]
	}

	refs, err := p.store.ReplaceDocumentChunks(ctx, document, plan)
	if err != nil {
		return fmt.Errorf("replace chunks: %w", err)
	}
	if err := p.syncSparseIndex(ctx, document, plan, refs); err != nil {
		return fmt.Errorf("sync sparse index: %w", err)
	}

	return nil
}

func buildEmbeddingInput(title, filename string, chunk store.ChunkInput) string {
	content := strings.TrimSpace(chunk.Content)
	if content == "" {
		return ""
	}

	lines := make([]string, 0, 4)
	if documentTitle := strings.TrimSpace(title); documentTitle != "" {
		lines = append(lines, "Title: "+documentTitle)
	}
	if name := strings.TrimSpace(filename); name != "" {
		lines = append(lines, "Document: "+name)
	}
	if section, ok := chunk.Metadata["section"].(string); ok {
		section = strings.TrimSpace(section)
		if section != "" {
			lines = append(lines, "Section: "+section)
		}
	}
	if page, ok := metadataInt(chunk.Metadata["page"]); ok {
		lines = append(lines, "Page: "+strconv.Itoa(page))
	}
	lines = append(lines, content)
	return strings.Join(lines, "\n")
}

func metadataInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func (p *Processor) syncSparseIndex(ctx context.Context, document store.DocumentForIngestion, plan store.ChunkPlan, refs store.StoredChunkRefs) error {
	if p.sparse == nil || !p.sparse.Enabled() {
		return nil
	}

	indexedChunks := make([]sparsesearch.IndexedChunk, 0, len(plan.Chunks))
	for _, chunk := range plan.Chunks {
		metadata := cloneMetadataMap(chunk.Metadata)
		if parentID := refs.SectionIDs[chunk.ParentIndex]; parentID != "" {
			metadata["parent_id"] = parentID
		}
		chunkID := refs.ChunkIDs[chunk.Index]
		if chunkID == "" {
			return fmt.Errorf("missing stored chunk id for chunk_index=%d", chunk.Index)
		}

		indexedChunks = append(indexedChunks, sparsesearch.IndexedChunk{
			ChunkID:        chunkID,
			OrgID:          document.OrgID,
			DocumentID:     document.ID,
			DocTitle:       document.Title,
			DocFilename:    document.Filename,
			ChunkIndex:     chunk.Index,
			Content:        chunk.Content,
			Section:        metadataString(metadata, "section"),
			HeadingPath:    metadataString(metadata, "heading_path"),
			ChunkKind:      metadataString(metadata, "chunk_kind"),
			AllowedRoleIDs: document.AllowedRoleIDs,
			Status:         "ready",
			Metadata:       metadata,
		})
	}

	return p.sparse.ReplaceDocument(ctx, document, indexedChunks)
}

func cloneMetadataMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
