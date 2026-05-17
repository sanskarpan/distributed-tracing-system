package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

func CommonMiddleware(next http.Handler) http.Handler {
	return middleware.Logger(middleware.Recoverer(next))
}

// APIKeyAuth returns middleware that enforces Bearer token authentication when apiKey != "".
// The key is compared in constant time to prevent timing attacks.
// SSE endpoints pass the key via the ?apiKey= query parameter as browsers cannot set headers for EventSource.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			return next // auth disabled
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check Authorization: Bearer <key>
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token := auth[len("Bearer "):]
				if secureEqual(token, apiKey) {
					next.ServeHTTP(w, r)
					return
				}
			}
			// Allow key via query param for SSE (EventSource API limitation)
			if secureEqual(r.URL.Query().Get("apiKey"), apiKey) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		})
	}
}

// secureEqual compares two strings in constant time to prevent timing attacks.
func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// CORS middleware allows the frontend dev server to call the API.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Tenant-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
