// Package apiclient is an HTTP client for the futuresooth API service
// (cmd/server). It decodes responses into the shared internal/aggregate types,
// so there is one definition of View/ChartData across the whole module.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/aggregate"
)

// Client fetches curated views and chart data from the API service. Results are
// cached in memory with a TTL, since the underlying BLS data changes only ~twice
// a month — this keeps pages snappy and tolerates brief backend blips.
type Client struct {
	baseURL string
	http    *http.Client
	ttl     time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	value   any
	expires time.Time
}

// viewsResponse is the envelope of GET /api/views.
type viewsResponse struct {
	Views []aggregate.View `json:"views"`
}

// NewClient builds a Client for the given API base URL.
func NewClient(baseURL string, timeout, ttl time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
		ttl:     ttl,
		cache:   make(map[string]cacheEntry),
	}
}

// ListViews returns the curated views from GET /api/views.
func (c *Client) ListViews(ctx context.Context) ([]aggregate.View, error) {
	if v, ok := c.getCached("views"); ok {
		return v.([]aggregate.View), nil
	}
	var resp viewsResponse
	if err := c.getJSON(ctx, "/api/views", &resp); err != nil {
		return nil, err
	}
	c.setCached("views", resp.Views)
	return resp.Views, nil
}

// GetView returns the chart data for one view from GET /api/views/{key}.
func (c *Client) GetView(ctx context.Context, key string) (aggregate.ChartData, error) {
	ck := "view:" + key
	if v, ok := c.getCached(ck); ok {
		return v.(aggregate.ChartData), nil
	}
	var chart aggregate.ChartData
	if err := c.getJSON(ctx, "/api/views/"+url.PathEscape(key), &chart); err != nil {
		return aggregate.ChartData{}, err
	}
	c.setCached(ck, chart)
	return chart, nil
}

func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("api request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("reading api response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api %s returned %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decoding api response %s: %w", path, err)
	}
	return nil
}

func (c *Client) getCached(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *Client) setCached(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
}
