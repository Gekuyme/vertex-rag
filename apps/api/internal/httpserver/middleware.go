package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/auth"
	"github.com/Gekuyme/vertex-rag/apps/api/internal/store"
)

type contextKey string

const authUserContextKey contextKey = "auth_user"

func authMiddleware(dbStore *store.Store, tokenManager *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accessToken := extractBearerToken(r.Header.Get("Authorization"))
			if accessToken == "" {
				writeError(w, http.StatusUnauthorized, "missing access token")
				return
			}

			claims, err := tokenManager.ParseToken(accessToken, auth.TokenTypeAccess)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid access token")
				return
			}

			user, err := dbStore.GetUserByID(r.Context(), claims.UserID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusUnauthorized, "user not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to load user")
				return
			}

			if user.OrgID != claims.OrgID {
				writeError(w, http.StatusUnauthorized, "token organization mismatch")
				return
			}

			if user.Status != "active" {
				writeError(w, http.StatusForbidden, "inactive user")
				return
			}

			ctx := context.WithValue(r.Context(), authUserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := currentUser(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if !hasPermission(user.Permissions, permission) {
				writeError(w, http.StatusForbidden, "permission denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func currentUser(ctx context.Context) (store.User, bool) {
	user, ok := ctx.Value(authUserContextKey).(store.User)
	return user, ok
}

func hasPermission(permissions []string, permission string) bool {
	for _, existingPermission := range permissions {
		if existingPermission == permission {
			return true
		}
	}

	return false
}

func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	wrapped := handler
	for index := len(middlewares) - 1; index >= 0; index-- {
		wrapped = middlewares[index](wrapped)
	}

	return wrapped
}
