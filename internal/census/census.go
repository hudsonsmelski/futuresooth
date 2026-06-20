// Package census fetches U.S. population data from the Census Bureau API and
// aggregates it into synthetic bls.Series so it flows through the same cache ->
// merge -> export pipeline as every other source.
//
// Three products, all national (us:1):
//   - a population pyramid (age 5-year bands x sex) from the latest PEP
//     "charv" vintage snapshot,
//   - population by race over time from ACS 1-year (table B02001),
//   - population by Hispanic origin over time from ACS 1-year (table B03003).
//
// The Census API requires a key on every request; with no key the source is a
// no-op (the refresher stays resilient).
package census

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

const baseURL = "https://api.census.gov/data"

// pyramidVintage is the PEP "charv" vintage used for the age/sex snapshot.
const pyramidVintage = 2023

// Earliest ACS 1-year vintage per table: the program began in 2005, but the
// Hispanic-origin table B03003 was only published from 2009 (earlier years
// return "unknown variable"), so we don't probe before then.
const (
	raceStartYear = 2005
	hispStartYear = 2009
)

// Synthetic series IDs (the wire contract with aggregate.Views).
const (
	idPyramidMale   = "census-pyramid-male"
	idPyramidFemale = "census-pyramid-female"

	idRaceWhite  = "census-race-white"
	idRaceBlack  = "census-race-black"
	idRaceNative = "census-race-natives"
	idRaceAsian  = "census-race-asian"
	idRaceMulti  = "census-race-multi"

	idHispYes = "census-hisp-hispanic"
	idHispNo  = "census-hisp-nonhispanic"
)

// ageBand pairs a PEP "charv" AGE code with a sortable, human band label. Only
// the standard 5-year groups are listed; the dataset's many overlapping summary
// bands (single years, "16 and older", etc.) are intentionally excluded. Labels
// are zero-padded so they sort lexically into chronological order.
type ageBand struct{ code, label string }

var ageBands = []ageBand{
	{"0401", "00-04"}, {"0509", "05-09"}, {"1014", "10-14"}, {"1519", "15-19"},
	{"2024", "20-24"}, {"2529", "25-29"}, {"3034", "30-34"}, {"3539", "35-39"},
	{"4044", "40-44"}, {"4549", "45-49"}, {"5054", "50-54"}, {"5559", "55-59"},
	{"6064", "60-64"}, {"6569", "65-69"}, {"7074", "70-74"}, {"7579", "75-79"},
	{"8084", "80-84"}, {"8599", "85+"},
}

// Source fetches and aggregates Census data.
type Source struct {
	apiKey  string
	baseURL string
	http    *http.Client
	// existing reports already-cached series (e.g. cache.Get); nil means treat
	// the cache as cold. Census data is archival, so years already cached are
	// reused instead of re-fetched on every refresh.
	existing func(id string) (bls.Series, bool)
}

// New builds a Source. A zero timeout defaults to 30s. existing may be nil.
func New(apiKey string, timeout time.Duration, existing func(id string) (bls.Series, bool)) *Source {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Source{apiKey: apiKey, baseURL: baseURL, http: &http.Client{Timeout: timeout}, existing: existing}
}

// Fetch returns the pyramid snapshot plus race and Hispanic time series. It
// implements refresh.Source. With no API key it logs and returns nil.
func (s *Source) Fetch(ctx context.Context, startYear, endYear int) ([]bls.Series, error) {
	if s.apiKey == "" {
		log.Printf("census: no CENSUS_API_KEY set; skipping Census source")
		return nil, nil
	}
	fetchedAt := time.Now().UTC()

	var out []bls.Series

	pyramid, err := s.fetchPyramid(ctx, fetchedAt)
	if err != nil {
		return nil, fmt.Errorf("census pyramid: %w", err)
	}
	out = append(out, pyramid...)

	acs, err := s.fetchACS(ctx, startYear, endYear, fetchedAt)
	if err != nil {
		return nil, fmt.Errorf("census acs: %w", err)
	}
	out = append(out, acs...)

	return out, nil
}

// get performs one Census API GET and decodes the JSON 2-D array
// ([[header...],[row...],...]). A 404 is reported via ok=false (year not
// released) rather than an error.
func (s *Source) get(ctx context.Context, path string, q url.Values) (rows [][]string, ok bool, err error) {
	q.Set("key", s.apiKey)
	u := s.baseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, false, fmt.Errorf("decoding response: %w", err)
	}
	return rows, true, nil
}

// fetchPyramid builds the male/female age-band series from the PEP charv snapshot.
func (s *Source) fetchPyramid(ctx context.Context, fetchedAt time.Time) ([]bls.Series, error) {
	q := url.Values{}
	q.Set("get", "POP,AGE,SEX")
	q.Set("for", "us:1")
	q.Set("YEAR", strconv.Itoa(pyramidVintage))
	q.Set("POPGROUP", "001") // total population (all races)
	q.Set("HISP", "0")       // total (Hispanic and non-Hispanic)

	rows, ok, err := s.get(ctx, fmt.Sprintf("/%d/pep/charv", pyramidVintage), q)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("pep charv %d not found", pyramidVintage)
	}
	return pyramidSeries(rows, fetchedAt), nil
}

// fetchACS pulls race (B02001) and Hispanic origin (B03003) national counts for
// each ACS 1-year vintage in range. Race and Hispanic are fetched separately so
// that a year missing one table still contributes the other, and any per-year
// failure (unreleased vintage, table not yet published) is skipped rather than
// aborting the whole source.
func (s *Source) fetchACS(ctx context.Context, startYear, endYear int, fetchedAt time.Time) ([]bls.Series, error) {
	// Seed from the cache so already-fetched (archival) years are reused and only
	// missing years hit the API.
	race := map[string]map[int]float64{
		idRaceWhite: s.seed(idRaceWhite), idRaceBlack: s.seed(idRaceBlack),
		idRaceNative: s.seed(idRaceNative), idRaceAsian: s.seed(idRaceAsian),
		idRaceMulti: s.seed(idRaceMulti),
	}
	hisp := map[string]map[int]float64{idHispYes: s.seed(idHispYes), idHispNo: s.seed(idHispNo)}

	for y := yearMax(startYear, raceStartYear); y <= endYear; y++ {
		if _, have := race[idRaceWhite][y]; have {
			continue // archival year already cached
		}
		if v, ok := s.acsRow(ctx, y, "B02001_002E,B02001_003E,B02001_004E,B02001_005E,B02001_006E,B02001_008E"); ok {
			race[idRaceWhite][y] = num(v["B02001_002E"])
			race[idRaceBlack][y] = num(v["B02001_003E"])
			race[idRaceNative][y] = num(v["B02001_004E"]) + num(v["B02001_006E"]) // AIAN + NHPI
			race[idRaceAsian][y] = num(v["B02001_005E"])
			race[idRaceMulti][y] = num(v["B02001_008E"])
		}
	}
	for y := yearMax(startYear, hispStartYear); y <= endYear; y++ {
		if _, have := hisp[idHispNo][y]; have {
			continue
		}
		if v, ok := s.acsRow(ctx, y, "B03003_002E,B03003_003E"); ok {
			hisp[idHispNo][y] = num(v["B03003_002E"])
			hisp[idHispYes][y] = num(v["B03003_003E"])
		}
	}

	raceYears := sortedYearsOf(race[idRaceWhite])
	hispYears := sortedYearsOf(hisp[idHispNo])
	out := []bls.Series{
		yearSeries(idRaceWhite, "White", raceYears, race[idRaceWhite], fetchedAt),
		yearSeries(idRaceBlack, "Black", raceYears, race[idRaceBlack], fetchedAt),
		yearSeries(idRaceNative, "Natives", raceYears, race[idRaceNative], fetchedAt),
		yearSeries(idRaceAsian, "Asian", raceYears, race[idRaceAsian], fetchedAt),
		yearSeries(idRaceMulti, "Two or more", raceYears, race[idRaceMulti], fetchedAt),
		yearSeries(idHispYes, "Hispanic", hispYears, hisp[idHispYes], fetchedAt),
		yearSeries(idHispNo, "Non-Hispanic", hispYears, hisp[idHispNo], fetchedAt),
	}
	return out, nil
}

// seed reads a cached series' yearly points into a year->value map (empty when
// the series isn't cached or no lookup was provided).
func (s *Source) seed(id string) map[int]float64 {
	m := map[int]float64{}
	if s.existing == nil {
		return m
	}
	ser, ok := s.existing(id)
	if !ok {
		return m
	}
	for _, p := range ser.Points {
		if p.Value == nil {
			continue
		}
		if y, err := strconv.Atoi(p.Date); err == nil {
			m[y] = *p.Value
		}
	}
	return m
}

// acsRow fetches one ACS 1-year national row for the given comma-separated
// variables. A missing vintage or any request error is logged and reported as
// ok=false so the caller can skip that year without failing the whole pull.
func (s *Source) acsRow(ctx context.Context, year int, vars string) (map[string]string, bool) {
	q := url.Values{}
	q.Set("get", vars)
	q.Set("for", "us:1")
	rows, ok, err := s.get(ctx, fmt.Sprintf("/%d/acs/acs1", year), q)
	if err != nil {
		log.Printf("census: acs %d (%s): %v", year, vars, err)
		return nil, false
	}
	if !ok || len(rows) < 2 {
		return nil, false
	}
	return index(rows), true
}

// --- pure aggregation (unit-tested without network) ---

// pyramidSeries turns PEP charv rows (cols include POP, AGE, SEX) into a male and
// a female series whose points are keyed by age band, in band order.
func pyramidSeries(rows [][]string, fetchedAt time.Time) []bls.Series {
	col := header(rows)
	popI, ageI, sexI := col["POP"], col["AGE"], col["SEX"]

	// code -> band label, for the bands we keep.
	label := make(map[string]string, len(ageBands))
	for _, b := range ageBands {
		label[b.code] = b.label
	}

	male := map[string]float64{}
	female := map[string]float64{}
	for _, r := range rows[1:] {
		band, keep := label[field(r, ageI)]
		if !keep {
			continue
		}
		pop := num(field(r, popI))
		switch field(r, sexI) {
		case "1":
			male[band] = pop
		case "2":
			female[band] = pop
		}
	}

	return []bls.Series{
		bandSeries(idPyramidMale, "Male", male, fetchedAt),
		bandSeries(idPyramidFemale, "Female", female, fetchedAt),
	}
}

// bandSeries builds a series with one point per age band, in canonical order.
func bandSeries(id, lbl string, byBand map[string]float64, fetchedAt time.Time) bls.Series {
	pts := make([]bls.Point, 0, len(ageBands))
	for _, b := range ageBands {
		v := byBand[b.label]
		pts = append(pts, bls.Point{Date: b.label, Value: &v})
	}
	return bls.Series{ID: id, Label: lbl, Units: "people", Points: pts, FetchedAt: fetchedAt}
}

// yearSeries builds a yearly series over the given years (0-filled for missing).
func yearSeries(id, lbl string, years []int, byYear map[int]float64, fetchedAt time.Time) bls.Series {
	pts := make([]bls.Point, 0, len(years))
	for _, y := range years {
		v := byYear[y]
		pts = append(pts, bls.Point{Date: strconv.Itoa(y), Value: &v})
	}
	return bls.Series{ID: id, Label: lbl, Units: "people", Points: pts, FetchedAt: fetchedAt}
}

// --- helpers ---

// header maps column name -> index from the first row.
func header(rows [][]string) map[string]int {
	m := map[string]int{}
	if len(rows) == 0 {
		return m
	}
	for i, name := range rows[0] {
		m[name] = i
	}
	return m
}

// index maps column name -> value for a single-data-row response (header + 1 row).
func index(rows [][]string) map[string]string {
	m := map[string]string{}
	if len(rows) < 2 {
		return m
	}
	for i, name := range rows[0] {
		if i < len(rows[1]) {
			m[name] = rows[1][i]
		}
	}
	return m
}

func field(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func yearMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortedYearsOf(m map[int]float64) []int {
	years := make([]int, 0, len(m))
	for y := range m {
		years = append(years, y)
	}
	sort.Ints(years)
	return years
}

func num(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
