package api

import (
	"net/http"
	"os"
)

// HandleConfig returns non-sensitive runtime configuration to the frontend.
// Currently exposes LOG_LINK_TEMPLATE for log correlation links.
func HandleConfig(w http.ResponseWriter, r *http.Request) {
	logLinkTemplate := os.Getenv("LOG_LINK_TEMPLATE")
	writeJSON(w, map[string]any{
		"logLinkTemplate": logLinkTemplate,
	})
}
