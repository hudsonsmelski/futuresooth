package aggregate

import (
	"testing"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

func ptr(f float64) *float64 { return &f }

// TestMergeAlignsOnDateUnion checks the core merge behavior: two series with
// different date coverage are aligned onto the union of dates, with nil filled
// where a series has no observation for a month.
func TestMergeAlignsOnDateUnion(t *testing.T) {
	men := bls.Series{
		ID: "LNS14000001", Label: "Men, 16+", SeasonallyAdjusted: true,
		Points: []bls.Point{
			{Date: "2025-01", Value: ptr(4.0)},
			{Date: "2025-02", Value: nil}, // unavailable month
			{Date: "2025-03", Value: ptr(4.2)},
		},
	}
	women := bls.Series{
		ID: "LNS14000002", Label: "Women, 16+", SeasonallyAdjusted: true,
		Points: []bls.Point{
			// no 2025-01 observation -> should align to nil
			{Date: "2025-02", Value: ptr(3.9)},
			{Date: "2025-03", Value: ptr(3.8)},
			{Date: "2025-04", Value: ptr(3.7)}, // extra month men lacks
		},
	}

	view := View{
		Key:       "unemployment-by-sex",
		Title:     "Unemployment Rate by Sex",
		Units:     "percent",
		SeriesIDs: []string{"LNS14000001", "LNS14000002"},
	}
	byID := map[string]bls.Series{men.ID: men, women.ID: women}

	chart := Merge(view, byID, "", "")

	wantX := []string{"2025-01", "2025-02", "2025-03", "2025-04"}
	if len(chart.X.Values) != len(wantX) {
		t.Fatalf("x axis = %v, want %v", chart.X.Values, wantX)
	}
	for i, d := range wantX {
		if chart.X.Values[i] != d {
			t.Errorf("x[%d] = %q, want %q", i, chart.X.Values[i], d)
		}
	}
	if chart.Meta.Points != 4 {
		t.Errorf("meta.points = %d, want 4", chart.Meta.Points)
	}
	if !chart.SeasonallyAdjusted {
		t.Errorf("expected SeasonallyAdjusted true when all series are SA")
	}

	if len(chart.Series) != 2 {
		t.Fatalf("got %d series, want 2", len(chart.Series))
	}

	// Men: aligned to [4.0, nil, 4.2, nil] (missing 2025-04, nil for 2025-02).
	menVals := chart.Series[0].Values
	if menVals[0] == nil || *menVals[0] != 4.0 {
		t.Errorf("men[2025-01] = %v, want 4.0", menVals[0])
	}
	if menVals[1] != nil {
		t.Errorf("men[2025-02] = %v, want nil (unavailable)", *menVals[1])
	}
	if menVals[3] != nil {
		t.Errorf("men[2025-04] = %v, want nil (no observation)", *menVals[3])
	}

	// Women: aligned to [nil, 3.9, 3.8, 3.7] (missing 2025-01).
	womenVals := chart.Series[1].Values
	if womenVals[0] != nil {
		t.Errorf("women[2025-01] = %v, want nil (no observation)", *womenVals[0])
	}
	if womenVals[3] == nil || *womenVals[3] != 3.7 {
		t.Errorf("women[2025-04] = %v, want 3.7", womenVals[3])
	}
}

func TestMergeRespectsRange(t *testing.T) {
	men := bls.Series{
		ID: "LNS14000001", Label: "Men, 16+", SeasonallyAdjusted: true,
		Points: []bls.Point{
			{Date: "2024-11", Value: ptr(4.0)},
			{Date: "2024-12", Value: ptr(4.1)},
			{Date: "2025-01", Value: ptr(4.2)},
			{Date: "2025-02", Value: ptr(4.3)},
		},
	}
	view := View{Key: "x", SeriesIDs: []string{"LNS14000001"}}
	byID := map[string]bls.Series{men.ID: men}

	chart := Merge(view, byID, "2024-12", "2025-01")
	wantX := []string{"2024-12", "2025-01"}
	if len(chart.X.Values) != 2 || chart.X.Values[0] != wantX[0] || chart.X.Values[1] != wantX[1] {
		t.Fatalf("range clip x = %v, want %v", chart.X.Values, wantX)
	}
}
