package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/queue"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/storage"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
	"github.com/google/uuid"
)

type Server struct {
	httpServer *http.Server
	store      *store.Store
	auth       *auth.Manager
	storage    *storage.Client
	queue      *queue.Client
	corsOrigin string
}

func New(addr string, dbStore *store.Store, tokenManager *auth.Manager, storageClient *storage.Client, queueClient *queue.Client, corsOrigin string) *Server {
	apiServer := &Server{
		store:      dbStore,
		auth:       tokenManager,
		storage:    storageClient,
		queue:      queueClient,
		corsOrigin: corsOrigin,
	}

	mux := http.NewServeMux()
	authMW := authMiddleware(dbStore, tokenManager)

	mux.HandleFunc("GET /healthz", apiServer.healthz)

	mux.HandleFunc("POST /auth/register_owner", apiServer.registerOwner)
	mux.HandleFunc("POST /auth/login", apiServer.login)
	mux.HandleFunc("POST /auth/refresh", apiServer.refresh)
	mux.HandleFunc("POST /auth/logout", apiServer.logout)

	mux.Handle("GET /me", chain(http.HandlerFunc(apiServer.me), authMW))
	mux.Handle("GET /roles", chain(http.HandlerFunc(apiServer.roles), authMW))

	mux.Handle("GET /admin/roles", chain(
		http.HandlerFunc(apiServer.listRoles),
		authMW,
		requirePermission(store.PermissionManageRoles),
	))
	mux.Handle("POST /admin/roles", chain(
		http.HandlerFunc(apiServer.createRole),
		authMW,
		requirePermission(store.PermissionManageRoles),
	))
	mux.Handle("GET /admin/users", chain(
		http.HandlerFunc(apiServer.listUsers),
		authMW,
		requirePermission(store.PermissionManageUsers),
	))
	mux.Handle("PATCH /admin/users/{id}/role", chain(
		http.HandlerFunc(apiServer.updateUserRole),
		authMW,
		requirePermission(store.PermissionManageUsers),
	))
	mux.Handle("GET /documents", chain(
		http.HandlerFunc(apiServer.listDocuments),
		authMW,
	))
	mux.Handle("POST /documents/upload", chain(
		http.HandlerFunc(apiServer.uploadDocument),
		authMW,
		requirePermission(store.PermissionUploadDocs),
	))

	return &Server{
		store:      dbStore,
		auth:       tokenManager,
		storage:    storageClient,
		queue:      queueClient,
		corsOrigin: corsOrigin,
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           apiServer.withCORS(mux),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]string{
		"service": "api",
		"status":  "ok",
	}
	_ = json.NewEncoder(w).Encode(response)
}

type registerOwnerRequest struct {
	OrganizationName string `json:"organization_name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type updateUserRoleRequest struct {
	RoleID int64 `json:"role_id"`
}

type authResponse struct {
	AccessToken string     `json:"access_token"`
	ExpiresIn   int64      `json:"expires_in"`
	User        store.User `json:"user"`
}

func (s *Server) registerOwner(w http.ResponseWriter, r *http.Request) {
	var payload registerOwnerRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(payload.OrganizationName) == "" {
		writeError(w, http.StatusBadRequest, "organization_name is required")
		return
	}
	if strings.TrimSpace(payload.Email) == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if len(payload.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	passwordHash, err := auth.HashPassword(payload.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user, err := s.store.CreateOrganizationWithOwner(
		r.Context(),
		payload.OrganizationName,
		payload.Email,
		passwordHash,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to create owner: %v", err))
		return
	}

	accessToken, refreshToken, err := s.auth.NewTokenPair(user.ID, user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	s.setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusCreated, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(s.auth.AccessTTL().Seconds()),
		User:        user,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var payload loginRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	record, err := s.store.GetUserByEmail(r.Context(), payload.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	matched, err := auth.VerifyPassword(record.PasswordHash, payload.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify password")
		return
	}
	if !matched {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if record.Status != "active" {
		writeError(w, http.StatusForbidden, "user is not active")
		return
	}

	accessToken, refreshToken, err := s.auth.NewTokenPair(record.ID, record.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	s.setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(s.auth.AccessTTL().Seconds()),
		User:        record.User,
	})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	refreshTokenCookie, err := r.Cookie("refresh_token")
	if err != nil || strings.TrimSpace(refreshTokenCookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "refresh token missing")
		return
	}

	claims, err := s.auth.ParseToken(refreshTokenCookie.Value, auth.TokenTypeRefresh)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	user, err := s.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "user is not active")
		return
	}

	accessToken, refreshToken, err := s.auth.NewTokenPair(user.ID, user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	s.setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int64(s.auth.AccessTTL().Seconds()),
		User:        user,
	})
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	s.clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) roles(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	roles, err := s.store.ListRoles(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]store.Role{"roles": roles})
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	roles, err := s.store.ListRoles(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]store.Role{"roles": roles})
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())

	var payload createRoleRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	role, err := s.store.CreateRole(r.Context(), user.OrgID, payload.Name, payload.Permissions)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to create role: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, role)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	users, err := s.store.ListUsers(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]store.User{"users": users})
}

func (s *Server) updateUserRole(w http.ResponseWriter, r *http.Request) {
	adminUser, _ := currentUser(r.Context())
	targetUserID := r.PathValue("id")
	if strings.TrimSpace(targetUserID) == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var payload updateUserRoleRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.RoleID <= 0 {
		writeError(w, http.StatusBadRequest, "role_id must be positive")
		return
	}

	err := s.store.UpdateUserRole(r.Context(), adminUser.OrgID, targetUserID, payload.RoleID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user or role not found in organization")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update user role")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	documents, err := s.store.ListDocuments(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]store.Document{"documents": documents})
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = fileHeader.Filename
	}

	contentType := normalizeContentType(fileHeader)
	roleIDs, err := parseRoleIDs(r.Form["allowed_role_ids"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	storageKey := fmt.Sprintf("%s/%s/%s", user.OrgID, time.Now().Format("2006/01/02"), uuid.NewString()+filepath.Ext(fileHeader.Filename))
	if err := s.storage.Upload(r.Context(), storageKey, file, contentType); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload file")
		return
	}

	document, err := s.store.CreateDocument(r.Context(), store.CreateDocumentParams{
		OrgID:          user.OrgID,
		Title:          title,
		Filename:       fileHeader.Filename,
		MIME:           contentType,
		StorageKey:     storageKey,
		AllowedRoleIDs: roleIDs,
		CreatedBy:      user.ID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to create document: %v", err))
		return
	}

	if err := s.queue.EnqueueDocumentIngest(r.Context(), document.ID); err != nil {
		_ = s.store.UpdateDocumentStatus(r.Context(), document.ID, "failed")
		writeError(w, http.StatusInternalServerError, "failed to schedule ingestion")
		return
	}

	writeJSON(w, http.StatusCreated, document)
}

func decodeJSONBody(r *http.Request, dst interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid json body: multiple payloads are not allowed")
	}

	return nil
}

func parseRoleIDs(values []string) ([]int64, error) {
	if len(values) == 0 {
		return nil, errors.New("allowed_role_ids is required")
	}

	roleIDs := make([]int64, 0, len(values))
	for _, rawValue := range values {
		for _, chunk := range strings.Split(rawValue, ",") {
			cleanValue := strings.TrimSpace(chunk)
			if cleanValue == "" {
				continue
			}

			roleID, err := strconv.ParseInt(cleanValue, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid role id: %s", cleanValue)
			}
			roleIDs = append(roleIDs, roleID)
		}
	}

	if len(roleIDs) == 0 {
		return nil, errors.New("allowed_role_ids must include at least one role")
	}

	return roleIDs, nil
}

func normalizeContentType(fileHeader *multipart.FileHeader) string {
	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   int(s.auth.RefreshTTL().Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.corsOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
