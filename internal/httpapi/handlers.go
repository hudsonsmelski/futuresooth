package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/hudsonsmelski/futuresooth/internal/aggregate"
	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// chartMaxAge is how long clients may cache chart responses. BLS releases
// monthly, so an hour is conservative.
const chartMaxAge = 3600

// RefreshFunc triggers an immediate refresh of all series. It returns the number
// of series refreshed.
type RefreshFunc func(ctx context.Context) (int, error)

// Store is the read interface the handlers need from the cache.
type Store interface {
	Get(id string) (bls.Series, bool)
}

// Handlers bundles the dependencies the HTTP handlers require.
type Handlers struct {
	store      Store
	refresh    RefreshFunc
	adminToken string
}

// NewHandlers constructs Handlers. adminToken may be empty to disable the admin
// refresh endpoint.
func NewHandlers(store Store, refresh RefreshFunc, adminToken string) *Handlers {
	return &Handlers{store: store, refresh: refresh, adminToken: adminToken}
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, 0, map[string]string{"status": "ok"})
}

// listViews returns the curated view registry.
func (h *Handlers) listViews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, chartMaxAge, map[string]any{"views": aggregate.Views})
}

// getView returns chart-ready merged data for one view.
func (h *Handlers) getView(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	view, ok := aggregate.ViewByKey(key)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown view: "+key)
		return
	}

	start, err := monthBound(r.URL.Query().Get("start"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start: "+err.Error())
		return
	}
	end, err := monthBound(r.URL.Query().Get("end"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end: "+err.Error())
		return
	}

	seriesByID := make(map[string]bls.Series, len(view.SeriesIDs))
	for _, id := range view.SeriesIDs {
		if s, ok := h.store.Get(id); ok {
			seriesByID[id] = s
		}
	}
	if len(seriesByID) == 0 {
		writeError(w, http.StatusServiceUnavailable, "no data cached yet for this view; try again shortly")
		return
	}

	chart := aggregate.Merge(view, seriesByID, start, end)
	writeJSON(w, http.StatusOK, chartMaxAge, chart)
}

// getSeries returns a single normalized series straight from the cache.
func (h *Handlers) getSeries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "series not cached: "+id)
		return
	}
	writeJSON(w, http.StatusOK, chartMaxAge, s)
}

// adminRefresh triggers an immediate refresh. Disabled when adminToken is empty.
func (h *Handlers) adminRefresh(w http.ResponseWriter, r *http.Request) {
	if h.adminToken == "" {
		writeError(w, http.StatusNotFound, "admin refresh disabled")
		return
	}
	if r.Header.Get("X-Admin-Token") != h.adminToken {
		writeError(w, http.StatusUnauthorized, "invalid admin token")
		return
	}
	n, err := h.refresh(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "refresh failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, 0, map[string]int{"refreshed": n})
}

// monthBound normalizes a start/end query param to "YYYY-MM". It accepts a bare
// year ("2015" -> "2015-01" for start, "2015-12" for end) or a full "YYYY-MM".
// An empty input returns "" (open bound).
func monthBound(v string, isEnd bool) (string, error) {
	if v == "" {
		return "", nil
	}
	switch len(v) {
	case 4: // bare year
		if _, err := strconv.Atoi(v); err != nil {
			return "", err
		}
		if isEnd {
			return v + "-12", nil
		}
		return v + "-01", nil
	case 7: // YYYY-MM
		if v[4] != '-' {
			return "", strconv.ErrSyntax
		}
		y, err1 := strconv.Atoi(v[:4])
		m, err2 := strconv.Atoi(v[5:])
		if err1 != nil || err2 != nil || y < 1900 || m < 1 || m > 12 {
			return "", strconv.ErrSyntax
		}
		return v, nil
	default:
		return "", strconv.ErrSyntax
	}
}
