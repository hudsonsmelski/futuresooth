// Package refresh keeps the cache current by periodically pulling series from
// BLS in the background. Handlers never call BLS inline; this is the only path
// that does, which keeps responses fast and BLS calls bounded.
package refresh

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// Fetcher pulls normalized series from upstream.
type Fetcher interface {
	FetchSeries(ctx context.Context, ids []string, startYear, endYear int) ([]bls.Series, error)
}

// Store receives refreshed series and reports cache freshness.
type Store interface {
	Put(series []bls.Series)
	OldestFetch() (time.Time, bool)
}

// Exporter persists the current cache to disk (descriptive JSON + CSV per view).
type Exporter interface {
	Write(ctx context.Context) error
}

// Refresher periodically refreshes a fixed set of series IDs.
type Refresher struct {
	fetcher   Fetcher
	store     Store
	exporter  Exporter
	ids       []string
	startYear int
	interval  time.Duration
}

// New builds a Refresher.
func New(fetcher Fetcher, store Store, exporter Exporter, ids []string, startYear int, interval time.Duration) *Refresher {
	return &Refresher{
		fetcher:   fetcher,
		store:     store,
		exporter:  exporter,
		ids:       ids,
		startYear: startYear,
		interval:  interval,
	}
}

// Once fetches all configured series over [startYear, current year], stores them
// in the cache, and writes the descriptive data files. It returns the number of
// series stored.
func (r *Refresher) Once(ctx context.Context) (int, error) {
	endYear := time.Now().UTC().Year()

	series, err := r.fetcher.FetchSeries(ctx, r.ids, r.startYear, endYear)
	if err != nil {
		return 0, err
	}
	r.store.Put(series)
	if err := r.exporter.Write(ctx); err != nil {
		return 0, fmt.Errorf("writing data files: %w", err)
	}
	return len(series), nil
}

// Run refreshes once at startup if the cache is stale (older than the interval)
// or empty, then on every tick until ctx is cancelled. It blocks, so call it in
// a goroutine.
func (r *Refresher) Run(ctx context.Context) {
	if r.staleOrEmpty() {
		r.refresh(ctx)
	} else {
		log.Printf("refresh: cache is fresh, skipping startup refresh")
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("refresh: stopping")
			return
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

func (r *Refresher) staleOrEmpty() bool {
	oldest, ok := r.store.OldestFetch()
	if !ok {
		return true
	}
	return time.Since(oldest) >= r.interval
}

func (r *Refresher) refresh(ctx context.Context) {
	start := time.Now()
	n, err := r.Once(ctx)
	if err != nil {
		log.Printf("refresh: failed: %v", err)
		return
	}
	log.Printf("refresh: stored %d series in %s", n, time.Since(start).Round(time.Millisecond))
}
