package bls

import (
	"testing"
	"time"
)

// fixture mirrors the real BLS response shape: newest-first ordering, an "M13"
// annual average that must be dropped, and a "-" value (data unavailable) that
// must become a nil point rather than zero.
func TestToSeries(t *testing.T) {
	raw := apiSeries{
		SeriesID: "LNS14000000",
		Data: []apiDatum{
			{Year: "2025", Period: "M13", Value: "4.3"}, // annual avg -> dropped
			{Year: "2025", Period: "M03", Value: "4.2"},
			{Year: "2025", Period: "M02", Value: "-"}, // unavailable -> nil
			{Year: "2025", Period: "M01", Value: "4.0"},
			{Year: "2024", Period: "M12", Value: "4.1"},
		},
	}

	fetched := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	s := toSeries(raw, fetched)

	if s.ID != "LNS14000000" {
		t.Fatalf("ID = %q", s.ID)
	}
	if s.Label != "All, 16+" {
		t.Errorf("Label = %q, want catalog label", s.Label)
	}
	if !s.FetchedAt.Equal(fetched) {
		t.Errorf("FetchedAt = %v", s.FetchedAt)
	}

	// M13 dropped -> 4 points, sorted ascending.
	wantDates := []string{"2024-12", "2025-01", "2025-02", "2025-03"}
	if len(s.Points) != len(wantDates) {
		t.Fatalf("got %d points, want %d: %+v", len(s.Points), len(wantDates), s.Points)
	}
	for i, d := range wantDates {
		if s.Points[i].Date != d {
			t.Errorf("point %d date = %q, want %q", i, s.Points[i].Date, d)
		}
	}

	// The "-" value (2025-02) must be nil.
	feb := s.Points[2]
	if feb.Date != "2025-02" {
		t.Fatalf("expected 2025-02 at index 2, got %q", feb.Date)
	}
	if feb.Value != nil {
		t.Errorf("2025-02 value = %v, want nil (unavailable)", *feb.Value)
	}

	// A normal value is parsed.
	if s.Points[1].Value == nil || *s.Points[1].Value != 4.0 {
		t.Errorf("2025-01 value = %v, want 4.0", s.Points[1].Value)
	}
}

func TestMonthFromPeriod(t *testing.T) {
	cases := []struct {
		in    string
		want  int
		valid bool
	}{
		{"M01", 1, true},
		{"M12", 12, true},
		{"M13", 0, false}, // annual average
		{"M00", 0, false},
		{"Q01", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := monthFromPeriod(c.in)
		if ok != c.valid || (ok && got != c.want) {
			t.Errorf("monthFromPeriod(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.valid)
		}
	}
}
