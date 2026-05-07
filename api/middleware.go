package api

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

func CommonMiddleware(next http.Handler) http.Handler {
	return middleware.Logger(middleware.Recoverer(next))
}

// CORS middleware allows the frontend dev server to call the API.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
