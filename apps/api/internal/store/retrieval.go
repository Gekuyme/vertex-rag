package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type RetrievalOptions struct {
	OrgID          string
	RoleID         int64
	Query          string
	QueryEmbedding []float32
	TopK           int
	CandidateK     int
	MaxPerDoc      int
}

type RetrievalChunk struct {
	ChunkID     string
	DocumentID  string
	DocTitle    string
	DocFilename string
	ChunkIndex  int
	Content     string
	Metadata    map[string]any
	VectorScore float64
	TextScore   float64
	Score       float64
}

func (s *Store) RetrieveChunks(ctx context.Context, opts RetrievalOptions) ([]RetrievalChunk, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	if strings.TrimSpace(opts.OrgID) == "" {
		return nil, errors.New("org_id is required")
	}
	if opts.RoleID <= 0 {
		return nil, errors.New("role_id must be positive")
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = 8
	}
	if topK > 30 {
		topK = 30
	}

	candidateK := opts.CandidateK
	if candidateK <= 0 {
		candidateK = 32
	}
	if candidateK < topK {
		candidateK = topK
	}
	if candidateK > 100 {
		candidateK = 100
	}

	maxPerDoc := opts.MaxPerDoc
	if maxPerDoc <= 0 || maxPerDoc > topK {
		maxPerDoc = topK
	}
	if maxPerDoc < 1 {
		maxPerDoc = 1
	}

	queryEmbedding := formatEmbeddingVector(opts.QueryEmbedding)

	rows, err := s.pool.Query(ctx, `
		WITH params AS (
			SELECT
				$1::uuid AS org_id,
				$2::bigint AS role_id,
				$3::text AS query_text,
				NULLIF($4::text, '')::vector AS query_embedding,
				$5::int AS candidate_k
		),
		vector_hits AS (
			SELECT
				dc.id AS chunk_id,
				dc.document_id,
				d.title AS doc_title,
				d.filename AS doc_filename,
				dc.chunk_index,
				dc.content,
				dc.metadata,
				ROW_NUMBER() OVER (ORDER BY dc.embedding <=> (SELECT query_embedding FROM params)) AS rank
			FROM document_chunks dc
			JOIN documents d ON d.id = dc.document_id
			WHERE dc.org_id = (SELECT org_id FROM params)
				AND d.status = 'ready'
				AND dc.allowed_role_ids @> ARRAY[(SELECT role_id FROM params)]::bigint[]
				AND dc.embedding IS NOT NULL
				AND (SELECT query_embedding FROM params) IS NOT NULL
			ORDER BY dc.embedding <=> (SELECT query_embedding FROM params)
			LIMIT (SELECT candidate_k FROM params)
		),
		text_hits AS (
			SELECT
				dc.id AS chunk_id,
				dc.document_id,
				d.title AS doc_title,
				d.filename AS doc_filename,
				dc.chunk_index,
				dc.content,
				dc.metadata,
				ROW_NUMBER() OVER (
					ORDER BY ts_rank_cd(dc.content_tsv, plainto_tsquery('simple', (SELECT query_text FROM params))) DESC
				) AS rank
			FROM document_chunks dc
			JOIN documents d ON d.id = dc.document_id
			WHERE dc.org_id = (SELECT org_id FROM params)
				AND d.status = 'ready'
				AND dc.allowed_role_ids @> ARRAY[(SELECT role_id FROM params)]::bigint[]
				AND dc.content_tsv @@ plainto_tsquery('simple', (SELECT query_text FROM params))
			ORDER BY ts_rank_cd(dc.content_tsv, plainto_tsquery('simple', (SELECT query_text FROM params))) DESC
			LIMIT (SELECT candidate_k FROM params)
		),
		merged AS (
			SELECT
				chunk_id,
				document_id,
				doc_title,
				doc_filename,
				chunk_index,
				content,
				metadata,
				1.0 / (60 + rank) AS vector_score,
				0.0::float8 AS text_score
			FROM vector_hits
			UNION ALL
			SELECT
				chunk_id,
				document_id,
				doc_title,
				doc_filename,
				chunk_index,
				content,
				metadata,
				0.0::float8 AS vector_score,
				1.0 / (60 + rank) AS text_score
			FROM text_hits
		),
			aggregated AS (
				SELECT
					chunk_id,
					document_id,
					doc_title,
					doc_filename,
					chunk_index,
					content,
					metadata,
					MAX(vector_score) AS vector_score,
					MAX(text_score) AS text_score,
					SUM(vector_score + text_score) AS score
				FROM merged
				GROUP BY chunk_id, document_id, doc_title, doc_filename, chunk_index, content, metadata
			),
			limited AS (
				SELECT
					*,
					ROW_NUMBER() OVER (PARTITION BY document_id ORDER BY score DESC, chunk_index ASC) AS doc_rank
				FROM aggregated
			)
			SELECT
				chunk_id::text,
				document_id::text,
				doc_title,
				doc_filename,
				chunk_index,
				content,
				metadata::text,
				vector_score,
				text_score,
				score
			FROM limited
			WHERE doc_rank <= $7
			ORDER BY score DESC, vector_score DESC, text_score DESC, chunk_index ASC
			LIMIT $6
		`, opts.OrgID, opts.RoleID, query, queryEmbedding, candidateK, topK, maxPerDoc)
	if err != nil {
		return nil, fmt.Errorf("retrieve chunks: %w", err)
	}
	defer rows.Close()

	chunks := make([]RetrievalChunk, 0)
	for rows.Next() {
		var chunk RetrievalChunk
		var metadataJSON []byte
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocumentID,
			&chunk.DocTitle,
			&chunk.DocFilename,
			&chunk.ChunkIndex,
			&chunk.Content,
			&metadataJSON,
			&chunk.VectorScore,
			&chunk.TextScore,
			&chunk.Score,
		); err != nil {
			return nil, fmt.Errorf("scan retrieval chunk: %w", err)
		}

		chunk.Metadata = map[string]any{}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &chunk.Metadata); err != nil {
				return nil, fmt.Errorf("decode retrieval metadata: %w", err)
			}
		}

		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retrieval chunks: %w", err)
	}

	return chunks, nil
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
