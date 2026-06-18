// Command web runs the futuresooth chart viewer: it renders pages server-side,
// fetching chart data from the API service so the browser stays same-origin.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/apiclient"
	"github.com/hudsonsmelski/futuresooth/internal/config"
	"github.com/hudsonsmelski/futuresooth/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[futuresooth-web] ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	client := apiclient.NewClient(cfg.BackendURL, cfg.RequestTimeout, cfg.CacheTTL)
	handlers := web.NewHandlers(client)
	srv := web.NewServer(cfg.FrontendAddr, handlers)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s (backend: %s)", cfg.FrontendAddr, cfg.BackendURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Printf("bye")
}
