package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

type DocumentForIngestion struct {
	ID             string
	OrgID          string
	StorageKey     string
	MIME           string
	Filename       string
	AllowedRoleIDs []int64
}

type ChunkInput struct {
	Index     int
	Content   string
	Metadata  map[string]any
	Embedding []float32
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) GetDocumentForIngestion(ctx context.Context, documentID string) (DocumentForIngestion, error) {
	var document DocumentForIngestion
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, storage_key, mime, filename, allowed_role_ids
		FROM documents
		WHERE id = $1
	`, documentID).Scan(
		&document.ID,
		&document.OrgID,
		&document.StorageKey,
		&document.MIME,
		&document.Filename,
		&document.AllowedRoleIDs,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentForIngestion{}, ErrNotFound
		}
		return DocumentForIngestion{}, fmt.Errorf("get document: %w", err)
	}

	return document, nil
}

func (s *Store) UpdateDocumentStatus(ctx context.Context, documentID, status string) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE documents
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, status, documentID)
	if err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) ReplaceDocumentChunks(ctx context.Context, document DocumentForIngestion, chunks []ChunkInput) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, document.ID); err != nil {
		return fmt.Errorf("delete existing chunks: %w", err)
	}

	for _, chunk := range chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		embeddingVector := formatEmbeddingVector(chunk.Embedding)

		_, err = tx.Exec(ctx, `
			INSERT INTO document_chunks (
				org_id,
				document_id,
				chunk_index,
				content,
				content_tsv,
				embedding,
				metadata,
				allowed_role_ids,
				created_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				to_tsvector('simple', $4),
				NULLIF($5, '')::vector,
				$6::jsonb,
				$7,
				$8
			)
		`,
			document.OrgID,
			document.ID,
			chunk.Index,
			chunk.Content,
			embeddingVector,
			metadataJSON,
			document.AllowedRoleIDs,
			time.Now(),
		)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *Store) IncrementOrganizationKBVersion(ctx context.Context, orgID string) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE organizations
		SET kb_version = kb_version + 1
		WHERE id = $1
	`, orgID)
	if err != nil {
		return fmt.Errorf("increment kb_version: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func formatEmbeddingVector(values []float32) string {
	if len(values) == 0 {
		return ""
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}

	return "[" + strings.Join(parts, ",") + "]"
}
