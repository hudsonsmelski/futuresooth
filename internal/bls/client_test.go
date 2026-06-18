package bls

import (
	"testing"
	"time"
)

func TestYearWindows(t *testing.T) {
	cases := []struct {
		name            string
		start, end, max int
		want            []window
	}{
		{"full history keyed", 1948, 2026, 20, []window{{1948, 1967}, {1968, 1987}, {1988, 2007}, {2008, 2026}}},
		{"exact multiple", 2000, 2019, 20, []window{{2000, 2019}}},
		{"single year", 2024, 2024, 20, []window{{2024, 2024}}},
		{"sub-window", 2020, 2024, 20, []window{{2020, 2024}}},
		{"keyless 10yr", 2000, 2024, 10, []window{{2000, 2009}, {2010, 2019}, {2020, 2024}}},
		{"reversed range", 2026, 2020, 20, []window{{2026, 2020}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := yearWindows(c.start, c.end, c.max)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("window %d = %v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestBuildSeriesAccumulatesWindows mirrors how FetchSeries stitches the points
// from two year windows for one series ID into a single ascending series.
func TestBuildSeriesAccumulatesWindows(t *testing.T) {
	// Two windows returned newest-first within each window, out of order overall.
	windowA := apiSeries{SeriesID: "LNS14000000", Data: []apiDatum{
		{Year: "1949", Period: "M02", Value: "4.7"},
		{Year: "1949", Period: "M01", Value: "4.3"},
		{Year: "1948", Period: "M13", Value: "3.8"}, // annual avg -> dropped
	}}
	windowB := apiSeries{SeriesID: "LNS14000000", Data: []apiDatum{
		{Year: "1969", Period: "M01", Value: "3.4"},
	}}

	var pts []Point
	pts = append(pts, parseData(windowA)...)
	pts = append(pts, parseData(windowB)...)
	s := buildSeries("LNS14000000", pts, time.Now())

	want := []string{"1949-01", "1949-02", "1969-01"}
	if len(s.Points) != len(want) {
		t.Fatalf("got %d points, want %d: %+v", len(s.Points), len(want), s.Points)
	}
	for i, d := range want {
		if s.Points[i].Date != d {
			t.Errorf("point %d = %q, want %q", i, s.Points[i].Date, d)
		}
	}
	if s.Label != "All, 16+" {
		t.Errorf("label = %q, want catalog label", s.Label)
	}
}
