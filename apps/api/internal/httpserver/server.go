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
	"github.com/Gekuyme/vertex-rag/apps/api/internal/embeddings"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/llm"
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
	embeddings embeddings.Provider
	llm        llm.Provider
	corsOrigin string
}

func New(
	addr string,
	dbStore *store.Store,
	tokenManager *auth.Manager,
	storageClient *storage.Client,
	queueClient *queue.Client,
	embeddingProvider embeddings.Provider,
	llmProvider llm.Provider,
	corsOrigin string,
) *Server {
	apiServer := &Server{
		store:      dbStore,
		auth:       tokenManager,
		storage:    storageClient,
		queue:      queueClient,
		embeddings: embeddingProvider,
		llm:        llmProvider,
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
	mux.Handle("GET /me/settings", chain(http.HandlerFunc(apiServer.getMySettings), authMW))
	mux.Handle("PATCH /me/settings", chain(http.HandlerFunc(apiServer.updateMySettings), authMW))
	mux.Handle("GET /roles", chain(http.HandlerFunc(apiServer.roles), authMW))
	mux.Handle("GET /chats", chain(http.HandlerFunc(apiServer.listChats), authMW))
	mux.Handle("POST /chats", chain(http.HandlerFunc(apiServer.createChat), authMW))
	mux.Handle("GET /chats/{id}/messages", chain(http.HandlerFunc(apiServer.listChatMessages), authMW))
	mux.Handle("POST /chats/{id}/messages", chain(http.HandlerFunc(apiServer.createChatMessage), authMW))

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
	mux.Handle("POST /admin/retrieval/debug", chain(
		http.HandlerFunc(apiServer.debugRetrieval),
		authMW,
		requirePermission(store.PermissionManageDocs),
	))

	return &Server{
		store:      dbStore,
		auth:       tokenManager,
		storage:    storageClient,
		queue:      queueClient,
		embeddings: embeddingProvider,
		llm:        llmProvider,
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

type updateMySettingsRequest struct {
	DefaultMode string `json:"default_mode"`
}

type createChatRequest struct {
	Title string `json:"title"`
}

type createChatMessageRequest struct {
	Content    string `json:"content"`
	Mode       string `json:"mode"`
	TopK       int    `json:"top_k"`
	CandidateK int    `json:"candidate_k"`
}

type retrievalDebugRequest struct {
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	CandidateK int    `json:"candidate_k"`
}

type retrievalCitation struct {
	ChunkID     string         `json:"chunk_id"`
	DocumentID  string         `json:"document_id"`
	DocTitle    string         `json:"doc_title"`
	DocFilename string         `json:"doc_filename"`
	Snippet     string         `json:"snippet"`
	Page        *int           `json:"page,omitempty"`
	Section     string         `json:"section,omitempty"`
	VectorScore float64        `json:"vector_score"`
	TextScore   float64        `json:"text_score"`
	Score       float64        `json:"score"`
	Metadata    map[string]any `json:"metadata"`
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

func (s *Server) getMySettings(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	settings, err := s.store.GetUserSettings(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) updateMySettings(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())

	var payload updateMySettingsRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mode := strings.ToLower(strings.TrimSpace(payload.DefaultMode))
	if mode == "" {
		writeError(w, http.StatusBadRequest, "default_mode is required")
		return
	}
	if err := store.ValidateMode(mode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if mode == store.ModeUnstrict && !hasPermission(user.Permissions, store.PermissionToggleWebSearch) {
		writeError(w, http.StatusForbidden, "unstrict mode is not allowed for this role")
		return
	}

	settings, err := s.store.UpsertUserSettings(r.Context(), user.ID, mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	writeJSON(w, http.StatusOK, settings)
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

func (s *Server) debugRetrieval(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())

	var payload retrievalDebugRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	query := strings.TrimSpace(payload.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	topK, candidateK := normalizeRetrievalLimits(payload.TopK, payload.CandidateK)

	vectors, err := s.embeddings.Embed(r.Context(), []string{query})
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to embed query: %v", err))
		return
	}
	if len(vectors) != 1 {
		writeError(w, http.StatusInternalServerError, "embedding provider returned unexpected result")
		return
	}

	results, err := s.store.RetrieveChunks(r.Context(), store.RetrievalOptions{
		OrgID:          user.OrgID,
		RoleID:         user.RoleID,
		Query:          query,
		QueryEmbedding: vectors[0],
		TopK:           topK,
		CandidateK:     candidateK,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve chunks: %v", err))
		return
	}

	citations := make([]retrievalCitation, 0, len(results))
	for _, result := range results {
		citations = append(citations, retrievalCitation{
			ChunkID:     result.ChunkID,
			DocumentID:  result.DocumentID,
			DocTitle:    result.DocTitle,
			DocFilename: result.DocFilename,
			Snippet:     truncateSnippet(result.Content, 320),
			Page:        metadataInt(result.Metadata, "page"),
			Section:     metadataString(result.Metadata, "section"),
			VectorScore: result.VectorScore,
			TextScore:   result.TextScore,
			Score:       result.Score,
			Metadata:    result.Metadata,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":       query,
		"top_k":       topK,
		"candidate_k": candidateK,
		"citations":   citations,
		"llm_context": buildLLMContext(results, 5000),
	})
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	chats, err := s.store.ListChats(r.Context(), user.OrgID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chats")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]store.Chat{"chats": chats})
}

func (s *Server) createChat(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())

	var payload createChatRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	chat, err := s.store.CreateChat(r.Context(), user.OrgID, user.ID, payload.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create chat")
		return
	}

	writeJSON(w, http.StatusCreated, chat)
}

func (s *Server) listChatMessages(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	chatID := strings.TrimSpace(r.PathValue("id"))
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "chat id is required")
		return
	}

	chat, err := s.store.GetChat(r.Context(), user.OrgID, chatID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load chat")
		return
	}
	if chat.CreatedBy != user.ID {
		writeError(w, http.StatusForbidden, "chat does not belong to current user")
		return
	}

	messages, err := s.store.ListChatMessages(r.Context(), user.OrgID, chatID, 300)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"chat":     chat,
		"messages": messages,
	})
}

func (s *Server) createChatMessage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	chatID := strings.TrimSpace(r.PathValue("id"))
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "chat id is required")
		return
	}

	chat, err := s.store.GetChat(r.Context(), user.OrgID, chatID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chat not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load chat")
		return
	}
	if chat.CreatedBy != user.ID {
		writeError(w, http.StatusForbidden, "chat does not belong to current user")
		return
	}

	var payload createChatMessageRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	query := strings.TrimSpace(payload.Content)
	if query == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	mode, err := s.resolveMode(r.Context(), user, payload.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if mode == store.ModeUnstrict && !hasPermission(user.Permissions, store.PermissionToggleWebSearch) {
		writeError(w, http.StatusForbidden, "unstrict mode is not allowed for this role")
		return
	}

	userID := user.ID
	userMessage, err := s.store.CreateMessage(r.Context(), store.CreateMessageParams{
		ChatID:    chatID,
		OrgID:     user.OrgID,
		UserID:    &userID,
		Role:      "user",
		Mode:      mode,
		Content:   query,
		Citations: []retrievalCitation{},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist user message")
		return
	}

	topK, candidateK := normalizeRetrievalLimits(payload.TopK, payload.CandidateK)
	vectors, err := s.embeddings.Embed(r.Context(), []string{query})
	if err != nil || len(vectors) != 1 {
		writeError(w, http.StatusBadRequest, "failed to embed query")
		return
	}

	retrieved, err := s.store.RetrieveChunks(r.Context(), store.RetrievalOptions{
		OrgID:          user.OrgID,
		RoleID:         user.RoleID,
		Query:          query,
		QueryEmbedding: vectors[0],
		TopK:           topK,
		CandidateK:     candidateK,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retrieve context")
		return
	}

	citations := make([]retrievalCitation, 0, len(retrieved))
	for _, result := range retrieved {
		citations = append(citations, retrievalCitation{
			ChunkID:     result.ChunkID,
			DocumentID:  result.DocumentID,
			DocTitle:    result.DocTitle,
			DocFilename: result.DocFilename,
			Snippet:     truncateSnippet(result.Content, 320),
			Page:        metadataInt(result.Metadata, "page"),
			Section:     metadataString(result.Metadata, "section"),
			VectorScore: result.VectorScore,
			TextScore:   result.TextScore,
			Score:       result.Score,
			Metadata:    result.Metadata,
		})
	}

	answer := s.buildFallbackAnswer(mode, retrieved)
	if !(mode == store.ModeStrict && len(retrieved) == 0) {
		contextText := buildLLMContext(retrieved, 7000)
		completion, completionErr := s.llm.Complete(r.Context(), llm.CompletionRequest{
			Messages: []llm.Message{
				{
					Role:    "system",
					Content: s.systemPromptForMode(mode),
				},
				{
					Role: "user",
					Content: fmt.Sprintf(
						"Вопрос пользователя:\n%s\n\nКонтекст:\n%s",
						query,
						contextText,
					),
				},
			},
			MaxTokens:   900,
			Temperature: s.temperatureForMode(mode),
		})
		if completionErr == nil && strings.TrimSpace(completion) != "" {
			answer = strings.TrimSpace(completion)
		} else if mode == store.ModeUnstrict {
			writeError(w, http.StatusBadGateway, "failed to generate assistant response")
			return
		}
	}

	assistantMessage, err := s.store.CreateMessage(r.Context(), store.CreateMessageParams{
		ChatID:    chatID,
		OrgID:     user.OrgID,
		UserID:    nil,
		Role:      "assistant",
		Mode:      mode,
		Content:   answer,
		Citations: citations,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist assistant message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mode":              mode,
		"user_message":      userMessage,
		"assistant_message": assistantMessage,
		"citations":         citations,
	})
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

func truncateSnippet(content string, maxRunes int) string {
	normalized := strings.TrimSpace(content)
	if normalized == "" {
		return ""
	}

	runes := []rune(normalized)
	if len(runes) <= maxRunes {
		return normalized
	}

	return string(runes[:maxRunes]) + "…"
}

func metadataInt(metadata map[string]any, key string) *int {
	rawValue, ok := metadata[key]
	if !ok {
		return nil
	}

	switch value := rawValue.(type) {
	case float64:
		converted := int(value)
		return &converted
	case int:
		converted := value
		return &converted
	default:
		return nil
	}
}

func metadataString(metadata map[string]any, key string) string {
	rawValue, ok := metadata[key]
	if !ok {
		return ""
	}

	value, ok := rawValue.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

func buildLLMContext(chunks []store.RetrievalChunk, maxChars int) string {
	if len(chunks) == 0 || maxChars <= 0 {
		return ""
	}

	var builder strings.Builder
	for index, chunk := range chunks {
		header := fmt.Sprintf(
			"[%d] %s (%s) chunk:%d\n",
			index+1,
			chunk.DocTitle,
			chunk.DocFilename,
			chunk.ChunkIndex,
		)
		content := strings.TrimSpace(chunk.Content) + "\n\n"

		if builder.Len()+len(header)+len(content) > maxChars {
			break
		}

		builder.WriteString(header)
		builder.WriteString(content)
	}

	return strings.TrimSpace(builder.String())
}

func normalizeRetrievalLimits(topK, candidateK int) (int, int) {
	if topK <= 0 {
		topK = 8
	}
	if topK > 30 {
		topK = 30
	}

	if candidateK <= 0 {
		candidateK = 32
	}
	if candidateK < topK {
		candidateK = topK
	}
	if candidateK > 100 {
		candidateK = 100
	}

	return topK, candidateK
}

func (s *Server) resolveMode(ctx context.Context, user store.User, requestedMode string) (string, error) {
	if strings.TrimSpace(requestedMode) != "" {
		mode := strings.ToLower(strings.TrimSpace(requestedMode))
		if err := store.ValidateMode(mode); err != nil {
			return "", err
		}
		return mode, nil
	}

	settings, err := s.store.GetUserSettings(ctx, user.ID)
	if err != nil {
		return "", fmt.Errorf("load user settings: %w", err)
	}
	if err := store.ValidateMode(settings.DefaultMode); err != nil {
		return "", err
	}

	return settings.DefaultMode, nil
}

func (s *Server) systemPromptForMode(mode string) string {
	if mode == store.ModeStrict {
		return "Ты корпоративный ассистент. Отвечай только на основе переданного контекста компании. Если контекста недостаточно, ответь дословно: \"Недостаточно данных в базе знаний.\" Не выдумывай факты."
	}

	return "Ты корпоративный ассистент. Используй контекст компании как приоритетный источник. Если контекст неполный, можешь дополнять общими знаниями и явно разделяй факты из контекста и общие рекомендации."
}

func (s *Server) temperatureForMode(mode string) float64 {
	if mode == store.ModeStrict {
		return 0.1
	}
	return 0.3
}

func (s *Server) buildFallbackAnswer(mode string, chunks []store.RetrievalChunk) string {
	if mode == store.ModeStrict && len(chunks) == 0 {
		return "Недостаточно данных в базе знаний."
	}
	if len(chunks) == 0 {
		return "Нет релевантных фрагментов базы знаний. Задайте вопрос точнее или загрузите документы."
	}

	snippets := make([]string, 0, minInt(2, len(chunks)))
	for index := 0; index < len(chunks) && index < 2; index++ {
		snippets = append(snippets, truncateSnippet(chunks[index].Content, 240))
	}

	return "Нашел релевантные фрагменты в базе знаний:\n- " + strings.Join(snippets, "\n- ")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
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
