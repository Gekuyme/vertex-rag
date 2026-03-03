package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

const (
	PermissionUploadDocs      = "can_upload_docs"
	PermissionManageUsers     = "can_manage_users"
	PermissionManageRoles     = "can_manage_roles"
	PermissionManageDocs      = "can_manage_documents"
	PermissionToggleWebSearch = "can_toggle_web_search"
	PermissionUseUnstrict     = "can_use_unstrict"
)

type Store struct {
	pool *pgxpool.Pool
}

type Role struct {
	ID          int64     `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	IsDefault   bool      `json:"is_default"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

type User struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Email       string    `json:"email"`
	RoleID      int64     `json:"role_id"`
	RoleName    string    `json:"role_name"`
	Permissions []string  `json:"permissions"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserRecord struct {
	User
	PasswordHash string
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

func (s *Store) GetOrganizationKBVersion(ctx context.Context, orgID string) (int64, error) {
	var kbVersion int64
	err := s.pool.QueryRow(ctx, `
		SELECT kb_version
		FROM organizations
		WHERE id = $1
	`, orgID).Scan(&kbVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("get organization kb_version: %w", err)
	}

	return kbVersion, nil
}

func (s *Store) CreateOrganizationWithOwner(ctx context.Context, orgName, email, passwordHash string) (User, error) {
	orgName = strings.TrimSpace(orgName)
	email = strings.ToLower(strings.TrimSpace(email))

	if orgName == "" || email == "" {
		return User{}, errors.New("org name and email are required")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var orgID string
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name)
		VALUES ($1)
		RETURNING id
	`, orgName).Scan(&orgID)
	if err != nil {
		return User{}, fmt.Errorf("insert organization: %w", err)
	}

	defaultRoles := []struct {
		Name        string
		IsDefault   bool
		Permissions []string
	}{
		{
			Name:      "Owner",
			IsDefault: false,
			Permissions: []string{
				PermissionUploadDocs,
				PermissionManageUsers,
				PermissionManageRoles,
				PermissionManageDocs,
				PermissionToggleWebSearch,
				PermissionUseUnstrict,
			},
		},
		{
			Name:      "Admin",
			IsDefault: false,
			Permissions: []string{
				PermissionUploadDocs,
				PermissionManageUsers,
				PermissionManageRoles,
				PermissionManageDocs,
				PermissionToggleWebSearch,
				PermissionUseUnstrict,
			},
		},
		{
			Name:      "Member",
			IsDefault: true,
			Permissions: []string{
				PermissionUploadDocs,
			},
		},
		{
			Name:        "Viewer",
			IsDefault:   false,
			Permissions: []string{},
		},
	}

	var ownerRoleID int64
	for _, role := range defaultRoles {
		permissions, marshalErr := json.Marshal(role.Permissions)
		if marshalErr != nil {
			return User{}, fmt.Errorf("marshal role permissions: %w", marshalErr)
		}

		var insertedRoleID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO roles (org_id, name, is_default, permissions)
			VALUES ($1, $2, $3, $4::jsonb)
			RETURNING id
		`, orgID, role.Name, role.IsDefault, permissions).Scan(&insertedRoleID)
		if err != nil {
			return User{}, fmt.Errorf("insert role: %w", err)
		}

		if role.Name == "Owner" {
			ownerRoleID = insertedRoleID
		}
	}

	var user User
	var permissionsJSON []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO users (org_id, email, password_hash, role_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, org_id, email, role_id, status, created_at
	`, orgID, email, passwordHash, ownerRoleID).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.RoleID, &user.Status, &user.CreatedAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("insert owner user: %w", err)
	}

	err = tx.QueryRow(ctx, `
		SELECT name, permissions::text
		FROM roles
		WHERE id = $1
	`, ownerRoleID).Scan(&user.RoleName, &permissionsJSON)
	if err != nil {
		return User{}, fmt.Errorf("load owner role: %w", err)
	}

	if err := json.Unmarshal(permissionsJSON, &user.Permissions); err != nil {
		return User{}, fmt.Errorf("decode owner permissions: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit tx: %w", err)
	}

	return user, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var record UserRecord
	var permissionsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT
			u.id,
			u.org_id,
			u.email,
			u.password_hash,
			u.role_id,
			r.name,
			r.permissions::text,
			u.status,
			u.created_at
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.email = $1
	`, email).Scan(
		&record.ID,
		&record.OrgID,
		&record.Email,
		&record.PasswordHash,
		&record.RoleID,
		&record.RoleName,
		&permissionsJSON,
		&record.Status,
		&record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserRecord{}, ErrNotFound
		}
		return UserRecord{}, fmt.Errorf("query user by email: %w", err)
	}

	if err := json.Unmarshal(permissionsJSON, &record.Permissions); err != nil {
		return UserRecord{}, fmt.Errorf("decode permissions: %w", err)
	}

	return record, nil
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (User, error) {
	var user User
	var permissionsJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT
			u.id,
			u.org_id,
			u.email,
			u.role_id,
			r.name,
			r.permissions::text,
			u.status,
			u.created_at
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.id = $1
	`, userID).Scan(
		&user.ID,
		&user.OrgID,
		&user.Email,
		&user.RoleID,
		&user.RoleName,
		&permissionsJSON,
		&user.Status,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("query user by id: %w", err)
	}

	if err := json.Unmarshal(permissionsJSON, &user.Permissions); err != nil {
		return User{}, fmt.Errorf("decode permissions: %w", err)
	}

	return user, nil
}

func (s *Store) ListRoles(ctx context.Context, orgID string) ([]Role, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, is_default, permissions::text, created_at
		FROM roles
		WHERE org_id = $1
		ORDER BY id
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		var permissionsJSON []byte
		if err := rows.Scan(
			&role.ID,
			&role.OrgID,
			&role.Name,
			&role.IsDefault,
			&permissionsJSON,
			&role.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		if err := json.Unmarshal(permissionsJSON, &role.Permissions); err != nil {
			return nil, fmt.Errorf("decode role permissions: %w", err)
		}
		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}

	return roles, nil
}

func (s *Store) CreateRole(ctx context.Context, orgID, name string, permissions []string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("role name cannot be empty")
	}

	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return Role{}, fmt.Errorf("marshal permissions: %w", err)
	}

	var role Role
	var encodedPermissions []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO roles (org_id, name, is_default, permissions)
		VALUES ($1, $2, false, $3::jsonb)
		RETURNING id, org_id, name, is_default, permissions::text, created_at
	`, orgID, name, permissionsJSON).Scan(
		&role.ID,
		&role.OrgID,
		&role.Name,
		&role.IsDefault,
		&encodedPermissions,
		&role.CreatedAt,
	)
	if err != nil {
		return Role{}, fmt.Errorf("insert role: %w", err)
	}

	if err := json.Unmarshal(encodedPermissions, &role.Permissions); err != nil {
		return Role{}, fmt.Errorf("decode role permissions: %w", err)
	}

	return role, nil
}

func (s *Store) ListUsers(ctx context.Context, orgID string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			u.id,
			u.org_id,
			u.email,
			u.role_id,
			r.name,
			r.permissions::text,
			u.status,
			u.created_at
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.org_id = $1
		ORDER BY u.created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var user User
		var permissionsJSON []byte
		if err := rows.Scan(
			&user.ID,
			&user.OrgID,
			&user.Email,
			&user.RoleID,
			&user.RoleName,
			&permissionsJSON,
			&user.Status,
			&user.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}

		if err := json.Unmarshal(permissionsJSON, &user.Permissions); err != nil {
			return nil, fmt.Errorf("decode user permissions: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (s *Store) UpdateUserRole(ctx context.Context, orgID, userID string, roleID int64) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET role_id = $1, updated_at = NOW()
		WHERE id = $2
		  AND org_id = $3
		  AND EXISTS (
		    SELECT 1
		    FROM roles
		    WHERE id = $1
		      AND org_id = $3
		  )
	`, roleID, userID, orgID)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
