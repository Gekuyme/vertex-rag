package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Document struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	Title          string    `json:"title"`
	Filename       string    `json:"filename"`
	MIME           string    `json:"mime"`
	StorageKey     string    `json:"storage_key"`
	Status         string    `json:"status"`
	AllowedRoleIDs []int64   `json:"allowed_role_ids"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateDocumentParams struct {
	OrgID          string
	Title          string
	Filename       string
	MIME           string
	StorageKey     string
	AllowedRoleIDs []int64
	CreatedBy      string
}

func (s *Store) CreateDocument(ctx context.Context, params CreateDocumentParams) (Document, error) {
	if strings.TrimSpace(params.Title) == "" {
		return Document{}, errors.New("title is required")
	}
	if len(params.AllowedRoleIDs) == 0 {
		return Document{}, errors.New("at least one role is required")
	}

	validRoleIDs, err := s.GetRoleIDsForOrg(ctx, params.OrgID)
	if err != nil {
		return Document{}, err
	}

	validRoleMap := map[int64]struct{}{}
	for _, roleID := range validRoleIDs {
		validRoleMap[roleID] = struct{}{}
	}

	for _, roleID := range params.AllowedRoleIDs {
		if _, ok := validRoleMap[roleID]; !ok {
			return Document{}, fmt.Errorf("role %d does not belong to organization", roleID)
		}
	}

	var document Document
	err = s.pool.QueryRow(ctx, `
		INSERT INTO documents (
			org_id, title, filename, mime, storage_key, status, allowed_role_ids, created_by
		)
		VALUES ($1, $2, $3, $4, $5, 'uploaded', $6, $7)
		RETURNING id, org_id, title, filename, mime, storage_key, status, allowed_role_ids, created_by, created_at, updated_at
	`,
		params.OrgID,
		params.Title,
		params.Filename,
		params.MIME,
		params.StorageKey,
		params.AllowedRoleIDs,
		params.CreatedBy,
	).Scan(
		&document.ID,
		&document.OrgID,
		&document.Title,
		&document.Filename,
		&document.MIME,
		&document.StorageKey,
		&document.Status,
		&document.AllowedRoleIDs,
		&document.CreatedBy,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
	if err != nil {
		return Document{}, fmt.Errorf("insert document: %w", err)
	}

	return document, nil
}

func (s *Store) ListDocuments(ctx context.Context, orgID string) ([]Document, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, title, filename, mime, storage_key, status, allowed_role_ids, created_by, created_at, updated_at
		FROM documents
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	documents := make([]Document, 0)
	for rows.Next() {
		var document Document
		if err := rows.Scan(
			&document.ID,
			&document.OrgID,
			&document.Title,
			&document.Filename,
			&document.MIME,
			&document.StorageKey,
			&document.Status,
			&document.AllowedRoleIDs,
			&document.CreatedBy,
			&document.CreatedAt,
			&document.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		documents = append(documents, document)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}

	return documents, nil
}

func (s *Store) GetRoleIDsForOrg(ctx context.Context, orgID string) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM roles WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list role ids: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var roleID int64
		if err := rows.Scan(&roleID); err != nil {
			return nil, fmt.Errorf("scan role id: %w", err)
		}
		ids = append(ids, roleID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role ids: %w", err)
	}

	return ids, nil
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
