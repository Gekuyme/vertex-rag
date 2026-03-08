package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type DenseRetrievalOptions struct {
	OrgID          string
	RoleID         int64
	QueryEmbedding []float32
	MaxResults     int
	MaxPerDoc      int
	QueryVariant   string
}

func (s *Store) DenseRetrieveChunks(ctx context.Context, opts DenseRetrievalOptions) ([]RetrievalChunk, error) {
	if strings.TrimSpace(opts.OrgID) == "" {
		return nil, fmt.Errorf("org_id is required")
	}
	if opts.RoleID <= 0 {
		return nil, fmt.Errorf("role_id must be positive")
	}
	if len(opts.QueryEmbedding) == 0 {
		return nil, nil
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	maxPerDoc := opts.MaxPerDoc
	if maxPerDoc <= 0 {
		maxPerDoc = 6
	}

	queryEmbedding := formatEmbeddingVector(opts.QueryEmbedding)
	queryEmbeddingDims := len(opts.QueryEmbedding)
	rows, err := s.pool.Query(ctx, denseRetrievalQuerySQL(queryEmbeddingDims), opts.OrgID, opts.RoleID, queryEmbedding, maxResults, maxPerDoc)
	if err != nil {
		return nil, fmt.Errorf("dense retrieve chunks: %w", err)
	}
	defer rows.Close()

	chunks := make([]RetrievalChunk, 0)
	for rows.Next() {
		var chunk RetrievalChunk
		var metadataJSON []byte
		var parentMetadataJSON []byte
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocumentID,
			&chunk.DocTitle,
			&chunk.DocFilename,
			&chunk.ChunkIndex,
			&chunk.Content,
			&metadataJSON,
			&chunk.ParentSectionID,
			&chunk.ParentContent,
			&parentMetadataJSON,
			&chunk.VectorScore,
			&chunk.DenseRank,
		); err != nil {
			return nil, fmt.Errorf("scan dense retrieval chunk: %w", err)
		}
		chunk.QueryVariant = strings.TrimSpace(opts.QueryVariant)
		chunk.RetrieversUsed = []string{"dense"}
		chunk.Metadata = decodeMetadata(metadataJSON)
		chunk.ParentMetadata = decodeMetadata(parentMetadataJSON)
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dense retrieval chunks: %w", err)
	}

	return chunks, nil
}

func (s *Store) GetRetrievalChunksByIDs(ctx context.Context, orgID string, roleID int64, chunkIDs []string) ([]RetrievalChunk, error) {
	if strings.TrimSpace(orgID) == "" {
		return nil, fmt.Errorf("org_id is required")
	}
	if roleID <= 0 {
		return nil, fmt.Errorf("role_id must be positive")
	}
	if len(chunkIDs) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			dc.id::text,
			dc.document_id::text,
			d.title,
			d.filename,
			dc.chunk_index,
			dc.content,
			dc.metadata::text,
			COALESCE(ds.id::text, ''),
			COALESCE(ds.content, ''),
			COALESCE(ds.metadata::text, '{}')
		FROM document_chunks dc
		JOIN documents d ON d.id = dc.document_id
		LEFT JOIN document_sections ds ON ds.id = dc.parent_section_id
		WHERE dc.org_id = $1
			AND d.status = 'ready'
			AND dc.allowed_role_ids @> ARRAY[$2]::bigint[]
			AND dc.id::text = ANY($3::text[])
	`, orgID, roleID, chunkIDs)
	if err != nil {
		return nil, fmt.Errorf("get retrieval chunks by ids: %w", err)
	}
	defer rows.Close()

	chunks := make([]RetrievalChunk, 0, len(chunkIDs))
	for rows.Next() {
		var chunk RetrievalChunk
		var metadataJSON []byte
		var parentMetadataJSON []byte
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocumentID,
			&chunk.DocTitle,
			&chunk.DocFilename,
			&chunk.ChunkIndex,
			&chunk.Content,
			&metadataJSON,
			&chunk.ParentSectionID,
			&chunk.ParentContent,
			&parentMetadataJSON,
		); err != nil {
			return nil, fmt.Errorf("scan hydrated chunk: %w", err)
		}
		chunk.Metadata = decodeMetadata(metadataJSON)
		chunk.ParentMetadata = decodeMetadata(parentMetadataJSON)
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hydrated chunks: %w", err)
	}

	return chunks, nil
}

func denseRetrievalQuerySQL(queryEmbeddingDims int) string {
	queryEmbeddingExpr := "NULLIF($3::text, '')::vector"
	vectorDistanceExpr := "dc.embedding <=> (SELECT query_embedding FROM params)"
	vectorDimsPredicate := "vector_dims(dc.embedding) = vector_dims((SELECT query_embedding FROM params))"

	if queryEmbeddingDims > 0 {
		queryEmbeddingExpr = fmt.Sprintf("NULLIF($3::text, '')::vector(%d)", queryEmbeddingDims)
		vectorDistanceExpr = fmt.Sprintf("dc.embedding::vector(%d) <=> (SELECT query_embedding FROM params)", queryEmbeddingDims)
		vectorDimsPredicate = fmt.Sprintf("vector_dims(dc.embedding) = %d", queryEmbeddingDims)
	}

	return fmt.Sprintf(`
		WITH params AS (
			SELECT
				$1::uuid AS org_id,
				$2::bigint AS role_id,
				%s AS query_embedding,
				$4::int AS max_results,
				$5::int AS max_per_doc
		),
		ranked AS (
			SELECT
				dc.id::text AS chunk_id,
				dc.document_id::text AS document_id,
				d.title AS doc_title,
				d.filename AS doc_filename,
				dc.chunk_index,
				dc.content,
				dc.metadata,
				COALESCE(ds.id::text, '') AS parent_section_id,
				COALESCE(ds.content, '') AS parent_content,
				COALESCE(ds.metadata::text, '{}') AS parent_metadata,
				%s AS distance,
				ROW_NUMBER() OVER (
					PARTITION BY dc.document_id
					ORDER BY %s ASC, dc.chunk_index ASC, dc.id ASC
				) AS doc_rank,
				ROW_NUMBER() OVER (
					ORDER BY %s ASC, dc.chunk_index ASC, dc.id ASC
				) AS dense_rank
			FROM document_chunks dc
			JOIN documents d ON d.id = dc.document_id
			LEFT JOIN document_sections ds ON ds.id = dc.parent_section_id
			WHERE dc.org_id = (SELECT org_id FROM params)
				AND d.status = 'ready'
				AND dc.allowed_role_ids @> ARRAY[(SELECT role_id FROM params)]::bigint[]
				AND dc.embedding IS NOT NULL
				AND (SELECT query_embedding FROM params) IS NOT NULL
				AND %s
		)
		SELECT
			chunk_id,
			document_id,
			doc_title,
			doc_filename,
			chunk_index,
			content,
			metadata::text,
			parent_section_id,
			parent_content,
			parent_metadata,
			1.0 / (1.0 + distance) AS vector_score,
			dense_rank
		FROM ranked
		WHERE doc_rank <= (SELECT max_per_doc FROM params)
		ORDER BY dense_rank ASC
		LIMIT (SELECT max_results FROM params)
	`, queryEmbeddingExpr, vectorDistanceExpr, vectorDistanceExpr, vectorDistanceExpr, vectorDimsPredicate)
}

func decodeMetadata(data []byte) map[string]any {
	metadata := map[string]any{}
	if len(data) == 0 {
		return metadata
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return metadata
	}
	if err := json.Unmarshal([]byte(trimmed), &metadata); err != nil {
		return map[string]any{"_decode_error": err.Error(), "_raw": strconv.Quote(trimmed)}
	}
	return metadata
}
