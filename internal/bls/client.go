// Package bls is a client for the U.S. Bureau of Labor Statistics public API v2,
// plus a catalog of known series and normalization of the raw responses.
package bls

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const apiURL = "https://api.bls.gov/publicAPI/v2/timeseries/data/"

// Per-request limits, per BLS documentation. With a registration key (v2) the
// limits are higher; keyless callers get the smaller public limits.
const (
	batchKeyless = 25
	batchKeyed   = 50

	maxYearsKeyless = 10
	maxYearsKeyed   = 20
)

// Client fetches series from the BLS API.
type Client struct {
	apiKey string
	http   *http.Client
}

// NewClient builds a Client. An empty apiKey uses the public (keyless) limits.
func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: timeout},
	}
}

// batchSize returns the max series per request given key presence.
func (c *Client) batchSize() int {
	if c.apiKey != "" {
		return batchKeyed
	}
	return batchKeyless
}

// maxYears returns the max year span per request given key presence.
func (c *Client) maxYears() int {
	if c.apiKey != "" {
		return maxYearsKeyed
	}
	return maxYearsKeyless
}

type apiRequest struct {
	SeriesID        []string `json:"seriesid"`
	StartYear       string   `json:"startyear"`
	EndYear         string   `json:"endyear"`
	RegistrationKey string   `json:"registrationkey,omitempty"`
}

// window is an inclusive year range for a single request.
type window struct{ start, end int }

// FetchSeries fetches and normalizes the given series IDs over the inclusive year
// range [startYear, endYear]. Requests are chunked to respect both the series-per-
// request and years-per-request limits, and the resulting points are accumulated
// per series and merged. All returned series are sorted ascending by date.
func (c *Client) FetchSeries(ctx context.Context, ids []string, startYear, endYear int) ([]Series, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	fetchedAt := time.Now().UTC()

	// Accumulate points per series ID across every (series-batch × year-window).
	pointsByID := make(map[string][]Point, len(ids))
	for _, idChunk := range chunkStrings(ids, c.batchSize()) {
		for _, w := range yearWindows(startYear, endYear, c.maxYears()) {
			raw, err := c.fetchChunk(ctx, idChunk, w.start, w.end)
			if err != nil {
				return nil, err
			}
			for _, rs := range raw {
				pointsByID[rs.SeriesID] = append(pointsByID[rs.SeriesID], parseData(rs)...)
			}
		}
	}

	// Build sorted series, preserving the input order of ids.
	out := make([]Series, 0, len(pointsByID))
	for _, id := range ids {
		pts, ok := pointsByID[id]
		if !ok {
			continue
		}
		out = append(out, buildSeries(id, pts, fetchedAt))
		delete(pointsByID, id) // guard against duplicate ids in input
	}
	return out, nil
}

// yearWindows splits [start, end] into non-overlapping inclusive ranges of at
// most maxYears each, e.g. (1948, 2026, 20) -> {1948-1967, 1968-1987, 1988-2007,
// 2008-2026}. If start > end or maxYears <= 0 it returns a single window.
func yearWindows(start, end, maxYears int) []window {
	if maxYears <= 0 || start > end {
		return []window{{start, end}}
	}
	var ws []window
	for s := start; s <= end; s += maxYears {
		e := s + maxYears - 1
		if e > end {
			e = end
		}
		ws = append(ws, window{s, e})
	}
	return ws
}

func (c *Client) fetchChunk(ctx context.Context, ids []string, startYear, endYear int) ([]apiSeries, error) {
	body, err := json.Marshal(apiRequest{
		SeriesID:        ids,
		StartYear:       strconv.Itoa(startYear),
		EndYear:         strconv.Itoa(endYear),
		RegistrationKey: c.apiKey,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bls request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("reading bls response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bls http %d: %s", resp.StatusCode, truncate(string(data), 200))
	}

	var parsed apiResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decoding bls response: %w", err)
	}
	if parsed.Status != statusSucceeded {
		return nil, fmt.Errorf("bls status %q: %s", parsed.Status, strings.Join(parsed.Message, "; "))
	}
	return parsed.Results.Series, nil
}

func chunkStrings(s []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	var chunks [][]string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
