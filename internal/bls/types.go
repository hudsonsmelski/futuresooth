package bls

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Point is a single normalized monthly observation. Value is nil when BLS
// reports the value as unavailable (e.g. "-"), so clients can render a gap
// rather than a misleading zero.
type Point struct {
	Date  string   `json:"date"`  // "YYYY-MM"
	Value *float64 `json:"value"` // nil == unavailable
}

// Series is a normalized, ascending-by-date time series for one BLS series ID.
type Series struct {
	ID                 string    `json:"id"`
	Label              string    `json:"label"`
	Units              string    `json:"units"`
	SeasonallyAdjusted bool      `json:"seasonally_adjusted"`
	Points             []Point   `json:"points"`
	FetchedAt          time.Time `json:"fetched_at"`
}

// --- Raw BLS API v2 response shapes (only the fields we use) ---

type apiResponse struct {
	Status  string   `json:"status"`
	Message []string `json:"message"`
	Results struct {
		Series []apiSeries `json:"series"`
	} `json:"Results"`
}

type apiSeries struct {
	SeriesID string     `json:"seriesID"`
	Data     []apiDatum `json:"data"`
}

type apiDatum struct {
	Year   string `json:"year"`
	Period string `json:"period"` // "M01".."M12", "M13" (annual avg)
	Value  string `json:"value"`  // numeric, or "-" when unavailable
}

const statusSucceeded = "REQUEST_SUCCEEDED"

// parseData extracts monthly points from one raw BLS series, mapping
// non-numeric values (e.g. "-") to nil and dropping annual averages (M13). The
// returned points are NOT sorted; callers accumulate across year windows and
// sort once via buildSeries.
func parseData(raw apiSeries) []Point {
	pts := make([]Point, 0, len(raw.Data))
	for _, d := range raw.Data {
		month, ok := monthFromPeriod(d.Period)
		if !ok {
			continue // skip M13 annual average and anything non-monthly
		}
		pt := Point{Date: fmt.Sprintf("%s-%02d", d.Year, month)}
		if v, err := strconv.ParseFloat(d.Value, 64); err == nil {
			pt.Value = &v
		} // else: leave nil (e.g. "-")
		pts = append(pts, pt)
	}
	return pts
}

// buildSeries assembles a normalized Series from accumulated points: it attaches
// catalog metadata, stamps fetchedAt, and sorts points ascending by date.
func buildSeries(id string, pts []Point, fetchedAt time.Time) Series {
	meta := Lookup(id)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date < pts[j].Date })
	return Series{
		ID:                 id,
		Label:              meta.Label,
		Units:              meta.Units,
		SeasonallyAdjusted: meta.SeasonallyAdjusted,
		FetchedAt:          fetchedAt,
		Points:             pts,
	}
}

// toSeries converts one raw BLS series (single window) into a normalized Series.
// Retained as a convenience over parseData + buildSeries.
func toSeries(raw apiSeries, fetchedAt time.Time) Series {
	return buildSeries(raw.SeriesID, parseData(raw), fetchedAt)
}

// monthFromPeriod parses a BLS monthly period code ("M01".."M12") into 1..12.
// Returns ok=false for annual averages ("M13") or any other code.
func monthFromPeriod(period string) (int, bool) {
	if len(period) != 3 || period[0] != 'M' {
		return 0, false
	}
	n, err := strconv.Atoi(period[1:])
	if err != nil || n < 1 || n > 12 {
		return 0, false
	}
	return n, true
}
