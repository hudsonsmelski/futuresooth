// Package web wires the HTTP routes that render the chart viewer.
package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed static
var staticFS embed.FS

// NewServer builds an *http.Server with all routes and middleware attached.
func NewServer(addr string, h *Handlers) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /", h.index)
	mux.HandleFunc("GET /d/{dataset}", h.dataset)
	mux.HandleFunc("GET /v/{key}", h.view)

	// Serve embedded static assets under /static/.
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	return &http.Server{
		Addr:              addr,
		Handler:           recoverMW(logMW(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// index "/" is registered as a catch-all; reject anything that isn't the root so
// unknown paths get a 404 instead of the home page.
func (h *Handlers) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		h.renderError(w, http.StatusNotFound, "Not found", "That page doesn't exist.")
		return
	}
	h.indexPage(w, r)
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
				http.Error(w, "internal server error", http.StatusInternalServerError)
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
