// Package httpapi wires the HTTP routes that serve chart-ready JSON.
package httpapi

import (
	"log"
	"net/http"
	"time"
)

// NewServer builds an *http.Server with all routes and middleware attached.
func NewServer(addr string, h *Handlers) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /api/views", h.listViews)
	mux.HandleFunc("GET /api/views/{key}", h.getView)
	mux.HandleFunc("GET /api/series/{id}", h.getSeries)
	mux.HandleFunc("POST /admin/refresh", h.adminRefresh)

	return &http.Server{
		Addr:              addr,
		Handler:           recoverMW(logMW(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// logMW logs each request with method, path, status, and duration.
func logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// recoverMW turns a handler panic into a 500 instead of crashing the server.
func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusWriter captures the response status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
