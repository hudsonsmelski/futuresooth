package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetViewParsesAndCaches(t *testing.T) {
	const body = `{
      "key":"unemployment-by-sex","title":"Unemployment Rate by Sex","units":"percent",
      "source":"BLS","seasonally_adjusted":true,
      "x":{"label":"Month","values":["2025-01","2025-02"]},
      "series":[
        {"id":"LNS14000001","label":"Men, 16+","values":[4.1,null]},
        {"id":"LNS14000002","label":"Women, 16+","values":[4.0,4.2]}
      ],
      "meta":{"fetched_at":"2026-06-17T12:00:00Z","points":2}
    }`

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/views/unemployment-by-sex" {
			http.NotFound(w, r)
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second, time.Minute)

	chart, err := c.GetView(context.Background(), "unemployment-by-sex")
	if err != nil {
		t.Fatalf("GetView: %v", err)
	}
	if chart.Title != "Unemployment Rate by Sex" || len(chart.Series) != 2 {
		t.Fatalf("unexpected chart: %+v", chart)
	}
	// The null in the payload must decode to a nil pointer (a chart gap), not 0.
	if chart.Series[0].Values[1] != nil {
		t.Errorf("Men 2025-02 = %v, want nil", *chart.Series[0].Values[1])
	}
	if chart.Series[1].Values[1] == nil || *chart.Series[1].Values[1] != 4.2 {
		t.Errorf("Women 2025-02 = %v, want 4.2", chart.Series[1].Values[1])
	}

	// Second call should be served from the TTL cache (no extra backend hit).
	if _, err := c.GetView(context.Background(), "unemployment-by-sex"); err != nil {
		t.Fatalf("GetView (cached): %v", err)
	}
	if hits != 1 {
		t.Errorf("backend hits = %d, want 1 (second call should be cached)", hits)
	}
}

func TestGetViewErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second, time.Minute)
	if _, err := c.GetView(context.Background(), "missing"); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}
