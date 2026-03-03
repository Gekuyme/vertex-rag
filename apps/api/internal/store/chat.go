package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ModeStrict   = "strict"
	ModeUnstrict = "unstrict"
)

type UserSettings struct {
	UserID      string    `json:"user_id"`
	DefaultMode string    `json:"default_mode"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Chat struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	CreatedBy string    `json:"created_by"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessage struct {
	ID        string            `json:"id"`
	ChatID    string            `json:"chat_id"`
	OrgID     string            `json:"org_id"`
	UserID    *string           `json:"user_id,omitempty"`
	Role      string            `json:"role"`
	Mode      string            `json:"mode"`
	Content   string            `json:"content"`
	Citations []json.RawMessage `json:"citations"`
	CreatedAt time.Time         `json:"created_at"`
}

type CreateMessageParams struct {
	ChatID    string
	OrgID     string
	UserID    *string
	Role      string
	Mode      string
	Content   string
	Citations any
}

func ValidateMode(mode string) error {
	switch mode {
	case ModeStrict, ModeUnstrict:
		return nil
	default:
		return fmt.Errorf("unsupported mode: %s", mode)
	}
}

func (s *Store) GetUserSettings(ctx context.Context, userID string) (UserSettings, error) {
	var settings UserSettings
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, default_mode, updated_at
		FROM user_settings
		WHERE user_id = $1
	`, userID).Scan(&settings.UserID, &settings.DefaultMode, &settings.UpdatedAt)
	if err == nil {
		return settings, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return UserSettings{}, fmt.Errorf("get user settings: %w", err)
	}

	now := time.Now()
	return UserSettings{
		UserID:      userID,
		DefaultMode: ModeStrict,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) UpsertUserSettings(ctx context.Context, userID, defaultMode string) (UserSettings, error) {
	if err := ValidateMode(defaultMode); err != nil {
		return UserSettings{}, err
	}

	var settings UserSettings
	err := s.pool.QueryRow(ctx, `
		INSERT INTO user_settings (user_id, default_mode, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET default_mode = EXCLUDED.default_mode, updated_at = NOW()
		RETURNING user_id, default_mode, updated_at
	`, userID, defaultMode).Scan(&settings.UserID, &settings.DefaultMode, &settings.UpdatedAt)
	if err != nil {
		return UserSettings{}, fmt.Errorf("upsert user settings: %w", err)
	}

	return settings, nil
}

func (s *Store) CreateChat(ctx context.Context, orgID, createdBy, title string) (Chat, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New chat"
	}

	var chat Chat
	err := s.pool.QueryRow(ctx, `
		INSERT INTO chats (org_id, created_by, title)
		VALUES ($1, $2, $3)
		RETURNING id, org_id, created_by, title, created_at, updated_at
	`, orgID, createdBy, title).Scan(
		&chat.ID,
		&chat.OrgID,
		&chat.CreatedBy,
		&chat.Title,
		&chat.CreatedAt,
		&chat.UpdatedAt,
	)
	if err != nil {
		return Chat{}, fmt.Errorf("create chat: %w", err)
	}

	return chat, nil
}

func (s *Store) ListChats(ctx context.Context, orgID, userID string) ([]Chat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, created_by, title, created_at, updated_at
		FROM chats
		WHERE org_id = $1
		  AND created_by = $2
		ORDER BY updated_at DESC, created_at DESC
	`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	chats := make([]Chat, 0)
	for rows.Next() {
		var chat Chat
		if err := rows.Scan(
			&chat.ID,
			&chat.OrgID,
			&chat.CreatedBy,
			&chat.Title,
			&chat.CreatedAt,
			&chat.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		chats = append(chats, chat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chats: %w", err)
	}

	return chats, nil
}

func (s *Store) GetChat(ctx context.Context, orgID, chatID string) (Chat, error) {
	var chat Chat
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, created_by, title, created_at, updated_at
		FROM chats
		WHERE id = $1
		  AND org_id = $2
	`, chatID, orgID).Scan(
		&chat.ID,
		&chat.OrgID,
		&chat.CreatedBy,
		&chat.Title,
		&chat.CreatedAt,
		&chat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Chat{}, ErrNotFound
		}
		return Chat{}, fmt.Errorf("get chat: %w", err)
	}

	return chat, nil
}

func (s *Store) DeleteChat(ctx context.Context, orgID, chatID string) error {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM chats
		WHERE id = $1
		  AND org_id = $2
	`, chatID, orgID)
	if err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) ListChatMessages(ctx context.Context, orgID, chatID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, chat_id, org_id, user_id, role, mode, content, citations::text, created_at
		FROM messages
		WHERE org_id = $1
		  AND chat_id = $2
		ORDER BY created_at ASC
		LIMIT $3
	`, orgID, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()

	messages := make([]ChatMessage, 0)
	for rows.Next() {
		var message ChatMessage
		var citationsJSON []byte
		if err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.OrgID,
			&message.UserID,
			&message.Role,
			&message.Mode,
			&message.Content,
			&citationsJSON,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}

		if len(citationsJSON) == 0 {
			message.Citations = []json.RawMessage{}
		} else if err := json.Unmarshal(citationsJSON, &message.Citations); err != nil {
			return nil, fmt.Errorf("decode chat message citations: %w", err)
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}

	return messages, nil
}

func (s *Store) CreateMessage(ctx context.Context, params CreateMessageParams) (ChatMessage, error) {
	if strings.TrimSpace(params.Content) == "" {
		return ChatMessage{}, errors.New("message content is required")
	}
	if err := ValidateMode(params.Mode); err != nil {
		return ChatMessage{}, err
	}
	if params.Role != "user" && params.Role != "assistant" {
		return ChatMessage{}, errors.New("message role must be user or assistant")
	}

	citationsJSON, err := json.Marshal(params.Citations)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("marshal citations: %w", err)
	}

	var message ChatMessage
	var rawCitations []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO messages (chat_id, org_id, user_id, role, mode, content, citations)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		RETURNING id, chat_id, org_id, user_id, role, mode, content, citations::text, created_at
	`, params.ChatID, params.OrgID, params.UserID, params.Role, params.Mode, params.Content, citationsJSON).Scan(
		&message.ID,
		&message.ChatID,
		&message.OrgID,
		&message.UserID,
		&message.Role,
		&message.Mode,
		&message.Content,
		&rawCitations,
		&message.CreatedAt,
	)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("create message: %w", err)
	}

	if len(rawCitations) == 0 {
		message.Citations = []json.RawMessage{}
	} else if err := json.Unmarshal(rawCitations, &message.Citations); err != nil {
		return ChatMessage{}, fmt.Errorf("decode message citations: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		UPDATE chats
		SET updated_at = NOW()
		WHERE id = $1
	`, params.ChatID)

	return message, nil
}
