// Package handlers contains the HTTP handlers for the job-service REST API.
package handlers

import (
	"encoding/json"
	"net/http"
)

// respondJSON writes v as a JSON-encoded HTTP response with the given status code.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// respondError writes a JSON error body in the form {"error": msg} with the given status code.
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}
