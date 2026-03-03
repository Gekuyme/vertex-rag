package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

type requestRateLimiter struct {
	limit  int
	burst  int
	window time.Duration

	mu      sync.Mutex
	entries map[string]rateLimitEntry
}

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

func newRequestRateLimiter(limit, burst int, window time.Duration) *requestRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	if limit <= 0 {
		return nil
	}
	if burst < 1 {
		burst = 1
	}
	return &requestRateLimiter{
		limit:   limit,
		burst:   burst,
		window:  window,
		entries: map[string]rateLimitEntry{},
	}
}

func (l *requestRateLimiter) allow(key string, now time.Time) (bool, int) {
	if l == nil {
		return true, 0
	}
	if key == "" {
		key = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateLimitEntry{
			windowStart: now,
			count:       0,
		}
	}

	maxRequests := l.limit + l.burst
	if entry.count >= maxRequests {
		retryAfter := int(l.window.Seconds() - now.Sub(entry.windowStart).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		l.entries[key] = entry
		return false, retryAfter
	}

	entry.count++
	l.entries[key] = entry

	if len(l.entries) > 5000 {
		for candidateKey, candidate := range l.entries {
			if now.Sub(candidate.windowStart) >= l.window*2 {
				delete(l.entries, candidateKey)
			}
		}
	}

	return true, 0
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			candidate := strings.TrimSpace(part)
			if candidate != "" {
				return candidate
			}
		}
	}

	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func rateLimitMiddleware(limiter *requestRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || r.Method == http.MethodOptions || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Method + ":" + r.URL.Path + ":" + clientIP(r)
			allowed, retryAfter := limiter.allow(key, time.Now())
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
