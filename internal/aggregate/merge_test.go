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

// TestMergeAnnualView checks that an annual view yields a "YYYY" axis labeled
// "Year", a "years" period, and uses the view's own Source attribution.
func TestMergeAnnualView(t *testing.T) {
	attempts := bls.Series{
		ID: "gcat-orbital-attempts", Label: "Attempts",
		Points: []bls.Point{
			{Date: "2023", Value: ptr(223)},
			{Date: "2024", Value: ptr(259)},
		},
	}
	view := View{
		Key:       "space-launches",
		Title:     "Orbital Launch Attempts",
		Units:     "launches",
		SeriesIDs: []string{"gcat-orbital-attempts"},
		Source:    "GCAT (J. McDowell, planet4589.org/space/gcat)",
		Annual:    true,
	}
	chart := Merge(view, map[string]bls.Series{attempts.ID: attempts}, "", "")

	if chart.X.Label != "Year" {
		t.Errorf("x.label = %q, want %q", chart.X.Label, "Year")
	}
	if chart.Meta.Period != "years" {
		t.Errorf("meta.period = %q, want %q", chart.Meta.Period, "years")
	}
	if chart.Source != view.Source {
		t.Errorf("source = %q, want %q", chart.Source, view.Source)
	}
	wantX := []string{"2023", "2024"}
	if len(chart.X.Values) != 2 || chart.X.Values[0] != wantX[0] || chart.X.Values[1] != wantX[1] {
		t.Fatalf("x = %v, want %v", chart.X.Values, wantX)
	}
}

// TestMergeRebase checks that a rebased view normalizes every series to 100 at
// the earliest month they all share, leaving relative growth intact.
func TestMergeRebase(t *testing.T) {
	a := bls.Series{
		ID: "A", Label: "A",
		Points: []bls.Point{
			{Date: "2000-01", Value: ptr(200)}, // base for A
			{Date: "2000-02", Value: ptr(220)}, // +10%
		},
	}
	b := bls.Series{
		ID: "B", Label: "B",
		Points: []bls.Point{
			// no 2000-01 -> common base is 2000-02
			{Date: "2000-02", Value: ptr(50)},
			{Date: "2000-03", Value: ptr(75)}, // +50% vs base
		},
	}
	view := View{Key: "x", SeriesIDs: []string{"A", "B"}, Rebase: true}
	chart := Merge(view, map[string]bls.Series{a.ID: a, b.ID: b}, "", "")

	// Axis: 2000-01, 2000-02, 2000-03. Common base = 2000-02 (index 1).
	av := chart.Series[0].Values
	bv := chart.Series[1].Values
	if av[1] == nil || *av[1] != 100 {
		t.Errorf("A at base = %v, want 100", av[1])
	}
	if bv[1] == nil || *bv[1] != 100 {
		t.Errorf("B at base = %v, want 100", bv[1])
	}
	if av[0] == nil || *av[0] != 90.909090909090907 { // 200/220*100
		t.Errorf("A before base = %v, want ~90.91", av[0])
	}
	if bv[2] == nil || *bv[2] != 150 { // 75/50*100
		t.Errorf("B after base = %v, want 150", bv[2])
	}
}

// TestMergePyramid checks that a pyramid view carries its chart type and an
// "Age" axis with the category x-values intact.
func TestMergePyramid(t *testing.T) {
	male := bls.Series{ID: "m", Label: "Male", Points: []bls.Point{
		{Date: "00-04", Value: ptr(9)}, {Date: "05-09", Value: ptr(10)},
	}}
	female := bls.Series{ID: "f", Label: "Female", Points: []bls.Point{
		{Date: "00-04", Value: ptr(8)}, {Date: "05-09", Value: ptr(9)},
	}}
	view := View{Key: "p", SeriesIDs: []string{"m", "f"}, Chart: "pyramid"}
	chart := Merge(view, map[string]bls.Series{"m": male, "f": female}, "", "")

	if chart.Chart != "pyramid" {
		t.Errorf("chart = %q, want pyramid", chart.Chart)
	}
	if chart.X.Label != "Age" || chart.Meta.Period != "age groups" {
		t.Errorf("axis=%q period=%q, want Age/age groups", chart.X.Label, chart.Meta.Period)
	}
	if len(chart.X.Values) != 2 || chart.X.Values[0] != "00-04" {
		t.Errorf("x = %v, want [00-04 05-09]", chart.X.Values)
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
