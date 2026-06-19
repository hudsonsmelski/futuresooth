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

// Source pulls a set of normalized series from one upstream (e.g. BLS or GCAT)
// over the inclusive year range [startYear, endYear].
type Source interface {
	Fetch(ctx context.Context, startYear, endYear int) ([]bls.Series, error)
}

// Store receives refreshed series and reports cache freshness/coverage.
type Store interface {
	Put(series []bls.Series)
	OldestFetch() (time.Time, bool)
	Has(id string) bool
}

// Exporter persists the current cache to disk (descriptive JSON + CSV per view).
type Exporter interface {
	Write(ctx context.Context) error
}

// Refresher periodically refreshes every configured Source.
type Refresher struct {
	sources   []Source
	store     Store
	exporter  Exporter
	expected  []string
	startYear int
	interval  time.Duration
}

// New builds a Refresher over the given sources. expected is the set of series
// IDs the views require; a startup refresh runs if any are missing from the
// cache, so newly-added series get fetched even when restored data is still
// "fresh".
func New(sources []Source, store Store, exporter Exporter, expected []string, startYear int, interval time.Duration) *Refresher {
	return &Refresher{
		sources:   sources,
		store:     store,
		exporter:  exporter,
		expected:  expected,
		startYear: startYear,
		interval:  interval,
	}
}

// Once fetches from every source over [startYear, current year], stores what was
// retrieved, and writes the descriptive data files. A single source failing is
// logged but does not block the others — only an all-sources failure (with no
// series retrieved) is returned as an error, so e.g. a GCAT outage can't wipe a
// successful BLS pull.
func (r *Refresher) Once(ctx context.Context) (int, error) {
	endYear := time.Now().UTC().Year()

	var all []bls.Series
	var firstErr error
	failures := 0
	for _, src := range r.sources {
		series, err := src.Fetch(ctx, r.startYear, endYear)
		if err != nil {
			failures++
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("refresh: source failed: %v", err)
			continue
		}
		all = append(all, series...)
	}

	if len(all) == 0 {
		if firstErr != nil {
			return 0, firstErr
		}
		return 0, nil
	}

	r.store.Put(all)
	if err := r.exporter.Write(ctx); err != nil {
		return 0, fmt.Errorf("writing data files: %w", err)
	}
	return len(all), nil
}

// Run refreshes once at startup if the cache is empty, stale (older than the
// interval), or missing a configured series, then on every tick until ctx is
// cancelled. It blocks, so call it in a goroutine.
func (r *Refresher) Run(ctx context.Context) {
	if r.needsStartupRefresh() {
		r.refresh(ctx)
	} else {
		log.Printf("refresh: cache is fresh and complete, skipping startup refresh")
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

func (r *Refresher) needsStartupRefresh() bool {
	// A series referenced by a view but absent from the cache (e.g. a dataset
	// added since the on-disk data was written) forces a refresh regardless of
	// how fresh the restored data is.
	for _, id := range r.expected {
		if !r.store.Has(id) {
			log.Printf("refresh: series %q not cached; refreshing at startup", id)
			return true
		}
	}
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
