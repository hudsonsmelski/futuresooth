// Package export persists curated views to disk as descriptive, human-friendly
// files: one chart-ready JSON and one wide CSV per view (e.g.
// unemployment_rate_by_sex.json / .csv). The CSV has a "month" column plus one
// column per series, so it opens and plots directly in a spreadsheet. These
// files are also the source the service restores from on boot.
package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hudsonsmelski/futuresooth/internal/aggregate"
	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// SeriesSource provides cached series by ID (satisfied by *cache.Cache).
type SeriesSource interface {
	Get(id string) (bls.Series, bool)
}

// Exporter writes and reads the per-view data files under a directory.
type Exporter struct {
	dir    string
	views  []aggregate.View
	source SeriesSource
}

// New creates an Exporter rooted at dir, creating the directory if needed.
func New(dir string, views []aggregate.View, source SeriesSource) (*Exporter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}
	return &Exporter{dir: dir, views: views, source: source}, nil
}

// Write regenerates the JSON and CSV files for every view from the current
// source data. Views with no cached series yet are skipped.
func (e *Exporter) Write(_ context.Context) error {
	for _, v := range e.views {
		byID := e.collect(v)
		if len(byID) == 0 {
			continue
		}
		chart := aggregate.Merge(v, byID, "", "")
		if err := e.writeJSON(v.FileBase(), chart); err != nil {
			return err
		}
		if err := e.writeCSV(v.FileBase(), chart); err != nil {
			return err
		}
	}
	return nil
}

// Load reads each view's JSON file back into normalized series, so the service
// can serve immediately after a restart without hitting BLS. Missing files are
// skipped (cold start). Series shared by multiple views are de-duplicated.
func (e *Exporter) Load(_ context.Context) ([]bls.Series, error) {
	byID := make(map[string]bls.Series)
	for _, v := range e.views {
		path := filepath.Join(e.dir, v.FileBase()+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var chart aggregate.ChartData
		if err := json.Unmarshal(data, &chart); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", path, err)
		}
		for _, s := range chartToSeries(chart) {
			byID[s.ID] = s
		}
	}
	out := make([]bls.Series, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	return out, nil
}

// collect gathers the cached series referenced by a view.
func (e *Exporter) collect(v aggregate.View) map[string]bls.Series {
	byID := make(map[string]bls.Series, len(v.SeriesIDs))
	for _, id := range v.SeriesIDs {
		if s, ok := e.source.Get(id); ok {
			byID[id] = s
		}
	}
	return byID
}

func (e *Exporter) writeJSON(base string, chart aggregate.ChartData) error {
	data, err := json.MarshalIndent(chart, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s.json: %w", base, err)
	}
	return e.writeFileAtomic(base+".json", data)
}

// writeCSV writes a wide CSV: a "month" column followed by one column per series
// (header = series label). Missing values are left as empty cells.
func (e *Exporter) writeCSV(base string, chart aggregate.ChartData) error {
	tmp := filepath.Join(e.dir, base+".csv.tmp")
	final := filepath.Join(e.dir, base+".csv")

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating %s.csv: %w", base, err)
	}
	w := csv.NewWriter(f)

	header := make([]string, 0, len(chart.Series)+1)
	header = append(header, "month")
	for _, s := range chart.Series {
		header = append(header, s.Label)
	}
	if err := w.Write(header); err != nil {
		f.Close()
		return err
	}

	for i, month := range chart.X.Values {
		row := make([]string, 0, len(chart.Series)+1)
		row = append(row, month)
		for _, s := range chart.Series {
			if v := s.Values[i]; v != nil {
				row = append(row, strconv.FormatFloat(*v, 'f', -1, 64))
			} else {
				row = append(row, "")
			}
		}
		if err := w.Write(row); err != nil {
			f.Close()
			return err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (e *Exporter) writeFileAtomic(name string, data []byte) error {
	final := filepath.Join(e.dir, name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	return os.Rename(tmp, final)
}

// chartToSeries reconstructs normalized series from a merged chart payload. All
// series in a view share the chart's units, seasonal-adjustment flag, and fetch
// time (they are always fetched together).
func chartToSeries(chart aggregate.ChartData) []bls.Series {
	out := make([]bls.Series, 0, len(chart.Series))
	for _, cs := range chart.Series {
		pts := make([]bls.Point, 0, len(chart.X.Values))
		for i, date := range chart.X.Values {
			var val *float64
			if i < len(cs.Values) {
				val = cs.Values[i]
			}
			pts = append(pts, bls.Point{Date: date, Value: val})
		}
		out = append(out, bls.Series{
			ID:                 cs.ID,
			Label:              cs.Label,
			Units:              chart.Units,
			SeasonallyAdjusted: chart.SeasonallyAdjusted,
			Points:             pts,
			FetchedAt:          chart.Meta.FetchedAt,
		})
	}
	return out
}
