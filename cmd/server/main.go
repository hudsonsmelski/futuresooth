// Command server runs the BLS unemployment data aggregator: it serves
// chart-ready JSON from an on-disk cache that a background worker keeps current.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/aggregate"
	"github.com/hudsonsmelski/futuresooth/internal/bls"
	"github.com/hudsonsmelski/futuresooth/internal/cache"
	"github.com/hudsonsmelski/futuresooth/internal/census"
	"github.com/hudsonsmelski/futuresooth/internal/config"
	"github.com/hudsonsmelski/futuresooth/internal/export"
	"github.com/hudsonsmelski/futuresooth/internal/gcat"
	"github.com/hudsonsmelski/futuresooth/internal/httpapi"
	"github.com/hudsonsmelski/futuresooth/internal/refresh"
)

// blsSource adapts the BLS client (which fetches a fixed ID list) to the
// refresh.Source interface.
type blsSource struct {
	client *bls.Client
	ids    []string
}

func (s blsSource) Fetch(ctx context.Context, startYear, endYear int) ([]bls.Series, error) {
	return s.client.FetchSeries(ctx, s.ids, startYear, endYear)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[futuresooth-api] ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if !cfg.HasAPIKey() {
		log.Printf("warning: no BLS_API_KEY set; using public (keyless) rate limits")
	}

	store := cache.New()

	// Exporter owns disk I/O: descriptive per-view JSON + CSV under DATA_DIR.
	exporter, err := export.New(cfg.DataDir, aggregate.Views, store)
	if err != nil {
		log.Fatalf("export: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Restore from the on-disk data files so we can serve immediately.
	restored, err := exporter.Load(ctx)
	if err != nil {
		log.Fatalf("restore from disk: %v", err)
	}
	store.Put(restored)
	if store.Empty() {
		log.Printf("cache is cold; background refresh will populate it")
	} else {
		log.Printf("cache restored with %d series from disk", len(store.All()))
	}

	client := bls.NewClient(cfg.BLSAPIKey, cfg.RequestTimeout)
	ids := bls.CatalogIDs()

	// Each source pulls its own series into the shared cache. BLS pulls the
	// unemployment + CPI catalog; GCAT pulls space-industry launch/satellite
	// data; Census pulls population data.
	sources := []refresh.Source{
		blsSource{client: client, ids: ids},
		gcat.New(0),
		census.New(cfg.CensusAPIKey, cfg.RequestTimeout, store.Get),
	}
	refresher := refresh.New(sources, store, exporter, aggregate.AllSeriesIDs(), cfg.StartYear, cfg.RefreshInterval)

	go refresher.Run(ctx)

	handlers := httpapi.NewHandlers(store, refresher.Once, cfg.AdminToken)
	srv := httpapi.NewServer(cfg.HTTPAddr, handlers)

	go func() {
		log.Printf("listening on %s (%d views, %d series in catalog)", cfg.HTTPAddr, len(aggregate.Views), len(ids))
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
