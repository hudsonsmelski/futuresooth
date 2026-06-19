// Package cache holds normalized BLS series in memory, keyed by series ID. It is
// the read source for HTTP handlers. Disk persistence lives in package export,
// which writes/reads the descriptive per-view data files.
package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// Cache is a concurrency-safe in-memory store of series keyed by series ID.
type Cache struct {
	lock   sync.RWMutex
	series map[string]bls.Series
}

// New creates an empty Cache.
func New() *Cache {
	return &Cache{series: make(map[string]bls.Series)}
}

// Get returns the series for id, if present.
func (c *Cache) Get(id string) (bls.Series, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	s, ok := c.series[id]
	return s, ok
}

// Has reports whether a series with the given ID is cached.
func (c *Cache) Has(id string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	_, ok := c.series[id]
	return ok
}

// All returns a snapshot of every cached series, sorted by ID.
func (c *Cache) All() []bls.Series {
	c.lock.RLock()
	defer c.lock.RUnlock()
	out := make([]bls.Series, 0, len(c.series))
	for _, s := range c.series {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Empty reports whether the cache holds no series.
func (c *Cache) Empty() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return len(c.series) == 0
}

// OldestFetch returns the earliest FetchedAt across all cached series and
// whether the cache holds anything. Used to decide if a refresh is due.
func (c *Cache) OldestFetch() (time.Time, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if len(c.series) == 0 {
		return time.Time{}, false
	}
	var oldest time.Time
	first := true
	for _, s := range c.series {
		if first || s.FetchedAt.Before(oldest) {
			oldest = s.FetchedAt
			first = false
		}
	}
	return oldest, true
}

// Put stores the given series, overwriting any existing entry with the same ID.
func (c *Cache) Put(series []bls.Series) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, s := range series {
		c.series[s.ID] = s
	}
}
