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
	Title          string
	StorageKey     string
	MIME           string
	Filename       string
	AllowedRoleIDs []int64
}

type SectionInput struct {
	Index       int
	HeadingPath string
	Content     string
	Metadata    map[string]any
}

type ChunkInput struct {
	Index       int
	ParentIndex int
	Content     string
	Metadata    map[string]any
	Embedding   []float32
}

type ChunkPlan struct {
	Sections []SectionInput
	Chunks   []ChunkInput
}

type StoredChunkRefs struct {
	SectionIDs map[int]string
	ChunkIDs   map[int]string
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
		SELECT id, org_id, title, storage_key, mime, filename, allowed_role_ids
		FROM documents
		WHERE id = $1
	`, documentID).Scan(
		&document.ID,
		&document.OrgID,
		&document.Title,
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

func (s *Store) ReplaceDocumentChunks(ctx context.Context, document DocumentForIngestion, plan ChunkPlan) (StoredChunkRefs, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StoredChunkRefs{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, document.ID); err != nil {
		return StoredChunkRefs{}, fmt.Errorf("delete existing chunks: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM document_sections WHERE document_id = $1`, document.ID); err != nil {
		return StoredChunkRefs{}, fmt.Errorf("delete existing sections: %w", err)
	}

	sectionIDs := make(map[int]string, len(plan.Sections))
	for _, section := range plan.Sections {
		metadataJSON, err := json.Marshal(section.Metadata)
		if err != nil {
			return StoredChunkRefs{}, fmt.Errorf("marshal section metadata: %w", err)
		}

		var sectionID string
		err = tx.QueryRow(ctx, `
			INSERT INTO document_sections (
				org_id,
				document_id,
				section_index,
				heading_path,
				content,
				metadata,
				allowed_role_ids,
				created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
			RETURNING id
		`,
			document.OrgID,
			document.ID,
			section.Index,
			section.HeadingPath,
			section.Content,
			metadataJSON,
			document.AllowedRoleIDs,
			time.Now(),
		).Scan(&sectionID)
		if err != nil {
			return StoredChunkRefs{}, fmt.Errorf("insert section: %w", err)
		}

		sectionIDs[section.Index] = sectionID
	}

	chunkIDs := make(map[int]string, len(plan.Chunks))
	for _, chunk := range plan.Chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return StoredChunkRefs{}, fmt.Errorf("marshal metadata: %w", err)
		}
		embeddingVector := formatEmbeddingVector(chunk.Embedding)
		parentSectionID := sectionIDs[chunk.ParentIndex]

		var chunkID string
		err = tx.QueryRow(ctx, `
			INSERT INTO document_chunks (
				org_id,
				document_id,
				parent_section_id,
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
				$5,
				to_tsvector('simple', $5),
				NULLIF($6, '')::vector,
				$7::jsonb,
				$8,
				$9
			)
			RETURNING id
		`,
			document.OrgID,
			document.ID,
			parentSectionID,
			chunk.Index,
			chunk.Content,
			embeddingVector,
			metadataJSON,
			document.AllowedRoleIDs,
			time.Now(),
		).Scan(&chunkID)
		if err != nil {
			return StoredChunkRefs{}, fmt.Errorf("insert chunk: %w", err)
		}
		chunkIDs[chunk.Index] = chunkID
	}

	if err := tx.Commit(ctx); err != nil {
		return StoredChunkRefs{}, fmt.Errorf("commit tx: %w", err)
	}

	return StoredChunkRefs{SectionIDs: sectionIDs, ChunkIDs: chunkIDs}, nil
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
