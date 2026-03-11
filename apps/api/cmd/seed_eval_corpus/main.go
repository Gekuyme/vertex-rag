package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/embeddings"
	apistore "github.com/Gekuyme/vertex-rag/apps/api/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type corpusFile struct {
	Documents []corpusDocument `json:"documents"`
}

type corpusDocument struct {
	Alias    string          `json:"alias"`
	Title    string          `json:"title"`
	Filename string          `json:"filename"`
	Sections []corpusSection `json:"sections"`
}

type corpusSection struct {
	Heading string `json:"heading"`
	Content string `json:"content"`
}

type seededChunk struct {
	SectionIndex int
	ChunkIndex   int
	Heading      string
	HeadingPath  string
	Content      string
	ChunkMeta    map[string]any
	Embedding    []float32
	ChunkID      string
}

type opensearchBulkChunk struct {
	ChunkID        string         `json:"chunk_id"`
	OrgID          string         `json:"org_id"`
	DocumentID     string         `json:"document_id"`
	DocTitle       string         `json:"doc_title"`
	DocFilename    string         `json:"doc_filename"`
	ChunkIndex     int            `json:"chunk_index"`
	Content        string         `json:"content"`
	Section        string         `json:"section"`
	HeadingPath    string         `json:"heading_path"`
	ChunkKind      string         `json:"chunk_kind"`
	AllowedRoleIDs []int64        `json:"allowed_role_ids"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata"`
}

func main() {
	corpusPath := flag.String("corpus", "docs/evals/retrieval_eval_corpus.json", "path to seeded eval corpus JSON")
	orgName := flag.String("org-name", "Eval Lab", "name of the seeded organization")
	ownerEmail := flag.String("owner-email", "eval@vertex.local", "seeded eval owner email")
	ownerPassword := flag.String("owner-password", "Password123!", "seeded eval owner password")
	reset := flag.Bool("reset", true, "delete existing org documents before seeding")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dbStore, err := apistore.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fatalf("open api store: %v", err)
	}
	defer dbStore.Close()

	rawPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fatalf("open raw pool: %v", err)
	}
	defer rawPool.Close()

	provider, err := embeddings.NewProvider(cfg.Embeddings)
	if err != nil {
		fatalf("init embeddings provider: %v", err)
	}

	orgUser, err := ensureEvalOwner(ctx, dbStore, *orgName, *ownerEmail, *ownerPassword)
	if err != nil {
		fatalf("ensure eval owner: %v", err)
	}

	roleIDs, err := dbStore.GetRoleIDsForOrg(ctx, orgUser.OrgID)
	if err != nil {
		fatalf("list role ids: %v", err)
	}
	if len(roleIDs) == 0 {
		fatalf("organization %s has no roles", orgUser.OrgID)
	}

	corpus, err := loadCorpus(*corpusPath)
	if err != nil {
		fatalf("load corpus: %v", err)
	}

	if *reset {
		if err := resetOrgDocuments(ctx, rawPool, cfg, orgUser.OrgID); err != nil {
			fatalf("reset eval corpus: %v", err)
		}
	}

	if err := ensureOpenSearchIndex(ctx, cfg); err != nil {
		fatalf("ensure opensearch index: %v", err)
	}

	seededDocs := 0
	seededChunks := 0
	for _, corpusDoc := range corpus.Documents {
		if err := validateCorpusDocument(corpusDoc); err != nil {
			fatalf("invalid corpus document %q: %v", corpusDoc.Alias, err)
		}

		document, err := dbStore.CreateDocument(ctx, apistore.CreateDocumentParams{
			OrgID:          orgUser.OrgID,
			Title:          corpusDoc.Title,
			Filename:       corpusDoc.Filename,
			MIME:           "text/markdown",
			StorageKey:     "seed/" + corpusDoc.Alias + ".md",
			AllowedRoleIDs: roleIDs,
			CreatedBy:      orgUser.ID,
		})
		if err != nil {
			fatalf("create document %q: %v", corpusDoc.Alias, err)
		}

		chunks, err := buildSeededChunks(ctx, provider, corpusDoc)
		if err != nil {
			fatalf("build chunks for %q: %v", corpusDoc.Alias, err)
		}
		if err := insertSectionsAndChunks(ctx, rawPool, document, corpusDoc, roleIDs, chunks); err != nil {
			fatalf("insert sections/chunks for %q: %v", corpusDoc.Alias, err)
		}
		if err := dbStore.UpdateDocumentStatus(ctx, document.ID, "ready"); err != nil {
			fatalf("mark document ready %q: %v", corpusDoc.Alias, err)
		}
		if err := indexDocumentChunks(ctx, cfg, document, roleIDs, chunks); err != nil {
			fatalf("index sparse chunks for %q: %v", corpusDoc.Alias, err)
		}

		seededDocs++
		seededChunks += len(chunks)
	}

	if _, err := rawPool.Exec(ctx, `UPDATE organizations SET kb_version = kb_version + 1 WHERE id = $1`, orgUser.OrgID); err != nil {
		fatalf("increment kb version: %v", err)
	}

	fmt.Printf("Seeded %d document(s) and %d chunk(s) into org %s for %s\n", seededDocs, seededChunks, orgUser.OrgID, orgUser.Email)
}

func loadCorpus(path string) (corpusFile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return corpusFile{}, fmt.Errorf("read corpus file: %w", err)
	}
	var corpus corpusFile
	if err := json.Unmarshal(contents, &corpus); err != nil {
		return corpusFile{}, fmt.Errorf("decode corpus file: %w", err)
	}
	if len(corpus.Documents) == 0 {
		return corpusFile{}, errors.New("corpus has no documents")
	}
	return corpus, nil
}

func validateCorpusDocument(document corpusDocument) error {
	if strings.TrimSpace(document.Alias) == "" {
		return errors.New("alias is required")
	}
	if strings.TrimSpace(document.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(document.Filename) == "" {
		return errors.New("filename is required")
	}
	if len(document.Sections) == 0 {
		return errors.New("at least one section is required")
	}
	return nil
}

func ensureEvalOwner(ctx context.Context, dbStore *apistore.Store, orgName, email, password string) (apistore.User, error) {
	record, err := dbStore.GetUserByEmail(ctx, email)
	if err == nil {
		return record.User, nil
	}
	if !errors.Is(err, apistore.ErrNotFound) {
		return apistore.User{}, err
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return apistore.User{}, fmt.Errorf("hash password: %w", err)
	}
	return dbStore.CreateOrganizationWithOwner(ctx, orgName, email, passwordHash)
}

func resetOrgDocuments(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, orgID string) error {
	rows, err := pool.Query(ctx, `SELECT id FROM documents WHERE org_id = $1`, orgID)
	if err != nil {
		return fmt.Errorf("list org documents: %w", err)
	}
	defer rows.Close()

	documentIDs := make([]string, 0)
	for rows.Next() {
		var documentID string
		if err := rows.Scan(&documentID); err != nil {
			return fmt.Errorf("scan document id: %w", err)
		}
		documentIDs = append(documentIDs, documentID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate document ids: %w", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM documents WHERE org_id = $1`, orgID); err != nil {
		return fmt.Errorf("delete org documents: %w", err)
	}
	for _, documentID := range documentIDs {
		if err := deleteFromOpenSearch(ctx, cfg, documentID); err != nil {
			return err
		}
	}
	return nil
}

func buildSeededChunks(ctx context.Context, provider embeddings.Provider, document corpusDocument) ([]seededChunk, error) {
	texts := make([]string, 0)
	chunks := make([]seededChunk, 0)
	chunkIndex := 0
	for sectionIndex, section := range document.Sections {
		sectionText := strings.TrimSpace(section.Content)
		if sectionText == "" {
			continue
		}
		heading := strings.TrimSpace(section.Heading)
		offset := 0
		for _, child := range splitIntoParagraphChunks(sectionText) {
			start, end := runeSpan(sectionText, child, offset)
			offset = end
			texts = append(texts, child)
			chunks = append(chunks, seededChunk{
				SectionIndex: sectionIndex,
				ChunkIndex:   chunkIndex,
				Heading:      heading,
				HeadingPath:  heading,
				Content:      child,
				ChunkMeta: map[string]any{
					"page":           1,
					"section":        heading,
					"heading_path":   heading,
					"chunk_kind":     "paragraph",
					"char_start":     start,
					"char_end":       end,
					"document_alias": document.Alias,
				},
			})
			chunkIndex++
		}
	}

	vectors, err := provider.Embed(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed corpus chunks: %w", err)
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("embedding provider returned %d vectors for %d chunks", len(vectors), len(chunks))
	}
	for index := range chunks {
		chunks[index].Embedding = vectors[index]
	}
	return chunks, nil
}

func splitIntoParagraphChunks(text string) []string {
	parts := strings.Split(text, "\n\n")
	chunks := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
	}
	if len(chunks) == 0 && strings.TrimSpace(text) != "" {
		return []string{strings.TrimSpace(text)}
	}
	return chunks
}

func runeSpan(sectionText, child string, offset int) (int, int) {
	sectionRunes := []rune(sectionText)
	childRunes := []rune(child)
	if len(childRunes) == 0 {
		return 0, 0
	}
	maxStart := len(sectionRunes) - len(childRunes)
	if maxStart < 0 {
		maxStart = 0
	}
	for start := offset; start <= maxStart; start++ {
		match := true
		for index, char := range childRunes {
			if sectionRunes[start+index] != char {
				match = false
				break
			}
		}
		if match {
			return start, start + len(childRunes)
		}
	}
	return 0, len(childRunes)
}

func insertSectionsAndChunks(ctx context.Context, pool *pgxpool.Pool, document apistore.Document, corpusDoc corpusDocument, roleIDs []int64, chunks []seededChunk) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sectionIDs := make(map[int]string, len(corpusDoc.Sections))
	for index, section := range corpusDoc.Sections {
		sectionText := strings.TrimSpace(section.Content)
		if sectionText == "" {
			continue
		}
		metaJSON, err := json.Marshal(map[string]any{
			"page":       1,
			"section":    strings.TrimSpace(section.Heading),
			"char_start": 0,
			"char_end":   len([]rune(sectionText)),
		})
		if err != nil {
			return fmt.Errorf("marshal section metadata: %w", err)
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
				allowed_role_ids
			)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
			RETURNING id
		`, document.OrgID, document.ID, index, strings.TrimSpace(section.Heading), sectionText, metaJSON, roleIDs).Scan(&sectionID)
		if err != nil {
			return fmt.Errorf("insert section: %w", err)
		}
		sectionIDs[index] = sectionID
	}

	for index := range chunks {
		metaJSON, err := json.Marshal(chunks[index].ChunkMeta)
		if err != nil {
			return fmt.Errorf("marshal chunk metadata: %w", err)
		}
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
				allowed_role_ids
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
				$8
			)
			RETURNING id
		`, document.OrgID, document.ID, sectionIDs[chunks[index].SectionIndex], chunks[index].ChunkIndex, chunks[index].Content, formatEmbeddingVector(chunks[index].Embedding), metaJSON, roleIDs).Scan(&chunkID)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		chunks[index].ChunkID = chunkID
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chunk tx: %w", err)
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

func ensureOpenSearchIndex(ctx context.Context, cfg config.Config) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.SparseSearch.Provider), "opensearch") || strings.TrimSpace(cfg.SparseSearch.URL) == "" {
		return nil
	}
	payload := map[string]any{
		"settings": map[string]any{
			"index": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"chunk_id":         map[string]any{"type": "keyword"},
				"org_id":           map[string]any{"type": "keyword"},
				"document_id":      map[string]any{"type": "keyword"},
				"doc_title":        map[string]any{"type": "text"},
				"doc_filename":     map[string]any{"type": "keyword"},
				"chunk_index":      map[string]any{"type": "integer"},
				"content":          map[string]any{"type": "text"},
				"section":          map[string]any{"type": "text"},
				"heading_path":     map[string]any{"type": "text"},
				"chunk_kind":       map[string]any{"type": "keyword"},
				"allowed_role_ids": map[string]any{"type": "long"},
				"status":           map[string]any{"type": "keyword"},
				"metadata":         map[string]any{"type": "object", "enabled": true},
			},
		},
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal opensearch mapping: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(cfg.SparseSearch.URL, "/")+"/"+cfg.SparseSearch.IndexName, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create opensearch ensure request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("ensure opensearch index: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if response.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "resource_already_exists_exception") {
		return nil
	}
	return fmt.Errorf("ensure opensearch index returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}

func deleteFromOpenSearch(ctx context.Context, cfg config.Config, documentID string) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.SparseSearch.Provider), "opensearch") || strings.TrimSpace(cfg.SparseSearch.URL) == "" || strings.TrimSpace(documentID) == "" {
		return nil
	}
	requestBody, err := json.Marshal(map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				"document_id": documentID,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal delete payload: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.SparseSearch.URL, "/")+"/"+cfg.SparseSearch.IndexName+"/_delete_by_query", bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("delete opensearch document: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return fmt.Errorf("delete opensearch document returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
}

func indexDocumentChunks(ctx context.Context, cfg config.Config, document apistore.Document, roleIDs []int64, chunks []seededChunk) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.SparseSearch.Provider), "opensearch") || strings.TrimSpace(cfg.SparseSearch.URL) == "" {
		return nil
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, chunk := range chunks {
		action := map[string]any{
			"index": map[string]any{
				"_index": cfg.SparseSearch.IndexName,
				"_id":    chunk.ChunkID,
			},
		}
		if err := encoder.Encode(action); err != nil {
			return fmt.Errorf("encode bulk action: %w", err)
		}
		payload := opensearchBulkChunk{
			ChunkID:        chunk.ChunkID,
			OrgID:          document.OrgID,
			DocumentID:     document.ID,
			DocTitle:       document.Title,
			DocFilename:    document.Filename,
			ChunkIndex:     chunk.ChunkIndex,
			Content:        chunk.Content,
			Section:        chunk.Heading,
			HeadingPath:    chunk.HeadingPath,
			ChunkKind:      "paragraph",
			AllowedRoleIDs: roleIDs,
			Status:         "ready",
			Metadata:       chunk.ChunkMeta,
		}
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("encode bulk document: %w", err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.SparseSearch.URL, "/")+"/_bulk", &body)
	if err != nil {
		return fmt.Errorf("create bulk request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-ndjson")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("bulk index opensearch chunks: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("bulk index returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
