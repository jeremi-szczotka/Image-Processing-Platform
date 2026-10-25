// Package handlers contains HTTP handler types and helper functions for the api-service.
package handlers

import (
	"encoding/json"
	"net/http"
)

// respondJSON writes v as a JSON body with the given HTTP status code.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// respondError writes a JSON {"error": msg} body with the given HTTP status code.
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}
