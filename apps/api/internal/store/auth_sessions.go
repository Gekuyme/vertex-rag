package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type AuthSession struct {
	ID               string
	UserID           string
	OrgID            string
	RefreshTokenHash string
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s *Store) CreateAuthSession(
	ctx context.Context,
	sessionID, userID, orgID, refreshTokenHash string,
	expiresAt time.Time,
) (AuthSession, error) {
	var session AuthSession
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth_sessions (id, user_id, org_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, org_id, refresh_token_hash, expires_at, last_used_at, revoked_at, created_at, updated_at
	`, sessionID, userID, orgID, refreshTokenHash, expiresAt).Scan(
		&session.ID,
		&session.UserID,
		&session.OrgID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.LastUsedAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return AuthSession{}, fmt.Errorf("create auth session: %w", err)
	}

	return session, nil
}

func (s *Store) GetAuthSession(ctx context.Context, sessionID string) (AuthSession, error) {
	var session AuthSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, org_id, refresh_token_hash, expires_at, last_used_at, revoked_at, created_at, updated_at
		FROM auth_sessions
		WHERE id = $1
	`, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.OrgID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.LastUsedAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthSession{}, ErrNotFound
		}
		return AuthSession{}, fmt.Errorf("get auth session: %w", err)
	}

	return session, nil
}

func (s *Store) RotateAuthSession(
	ctx context.Context,
	sessionID, refreshTokenHash string,
	expiresAt time.Time,
) (AuthSession, error) {
	var session AuthSession
	err := s.pool.QueryRow(ctx, `
		UPDATE auth_sessions
		SET refresh_token_hash = $2,
		    expires_at = $3,
		    last_used_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND revoked_at IS NULL
		RETURNING id, user_id, org_id, refresh_token_hash, expires_at, last_used_at, revoked_at, created_at, updated_at
	`, sessionID, refreshTokenHash, expiresAt).Scan(
		&session.ID,
		&session.UserID,
		&session.OrgID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.LastUsedAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthSession{}, ErrNotFound
		}
		return AuthSession{}, fmt.Errorf("rotate auth session: %w", err)
	}

	return session, nil
}

func (s *Store) RevokeAuthSession(ctx context.Context, sessionID string) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1
		  AND revoked_at IS NULL
	`, sessionID)
	if err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
