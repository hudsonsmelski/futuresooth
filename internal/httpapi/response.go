package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// writeJSON serializes v as JSON with the given status code and cache header.
func writeJSON(w http.ResponseWriter, status int, maxAgeSeconds int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if maxAgeSeconds > 0 {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAgeSeconds))
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encode error: %v", err)
	}
}

// errorBody is the JSON shape for error responses.
type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, 0, errorBody{Error: msg})
}
