package aggregate

import (
	"sort"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// Source is the attribution string included with every chart payload.
const Source = "U.S. Bureau of Labor Statistics"

// ChartData is the chart-ready JSON payload: a shared x-axis plus one entry per
// series, with values aligned to that axis (nil where a series has no data for
// that month, so a client can draw a gap).
type ChartData struct {
	Key                string        `json:"key"`
	Title              string        `json:"title"`
	Subtitle           string        `json:"subtitle,omitempty"`
	Units              string        `json:"units"`
	Source             string        `json:"source"`
	SeasonallyAdjusted bool          `json:"seasonally_adjusted"`
	X                  Axis          `json:"x"`
	Series             []ChartSeries `json:"series"`
	Meta               ChartMeta     `json:"meta"`
}

// Axis is the shared category axis (months as "YYYY-MM").
type Axis struct {
	Label  string   `json:"label"`
	Values []string `json:"values"`
}

// ChartSeries is one line: values are positionally aligned to Axis.Values.
type ChartSeries struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

// ChartMeta carries provenance for the merged payload.
type ChartMeta struct {
	FetchedAt time.Time `json:"fetched_at"`
	Points    int       `json:"points"`
}

// Merge aligns the view's series (looked up in seriesByID) onto the union of all
// their dates, producing chart-ready data. Optional start/end (inclusive,
// "YYYY-MM"; empty to ignore) clip the axis. Missing series are skipped.
func Merge(v View, seriesByID map[string]bls.Series, start, end string) ChartData {
	// Build the sorted union of dates within the optional [start, end] window.
	dateSet := make(map[string]struct{})
	for _, id := range v.SeriesIDs {
		s, ok := seriesByID[id]
		if !ok {
			continue
		}
		for _, p := range s.Points {
			if inRange(p.Date, start, end) {
				dateSet[p.Date] = struct{}{}
			}
		}
	}
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	// Index of date -> axis position for O(1) alignment.
	pos := make(map[string]int, len(dates))
	for i, d := range dates {
		pos[d] = i
	}

	out := ChartData{
		Key:      v.Key,
		Title:    v.Title,
		Subtitle: v.Subtitle,
		Units:    v.Units,
		Source:   Source,
		X:        Axis{Label: "Month", Values: dates},
		Meta:     ChartMeta{Points: len(dates)},
	}

	allSA := true
	anySeries := false
	var oldestFetch time.Time

	for _, id := range v.SeriesIDs {
		s, ok := seriesByID[id]
		if !ok {
			continue
		}
		anySeries = true
		if !s.SeasonallyAdjusted {
			allSA = false
		}
		if oldestFetch.IsZero() || s.FetchedAt.Before(oldestFetch) {
			oldestFetch = s.FetchedAt
		}

		values := make([]*float64, len(dates))
		for _, p := range s.Points {
			if i, ok := pos[p.Date]; ok {
				values[i] = p.Value
			}
		}
		out.Series = append(out.Series, ChartSeries{
			ID:     s.ID,
			Label:  s.Label,
			Values: values,
		})
	}

	out.SeasonallyAdjusted = anySeries && allSA
	out.Meta.FetchedAt = oldestFetch
	return out
}

// inRange reports whether date is within [start, end] (inclusive). Empty bounds
// are treated as open. All values are "YYYY-MM" so lexical compare works.
func inRange(date, start, end string) bool {
	if start != "" && date < start {
		return false
	}
	if end != "" && date > end {
		return false
	}
	return true
}
