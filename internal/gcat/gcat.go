// Package gcat fetches space-industry data from GCAT, Jonathan McDowell's
// General Catalog of Artificial Space Objects (planet4589.org/space/gcat, CC-BY).
//
// GCAT publishes periodically-updated, tab-separated data files rather than a
// live API, which suits this project's background refresher. This package
// downloads the relevant files, aggregates their event-level rows into yearly
// totals, and emits them as synthetic bls.Series so they flow through the same
// cache -> merge -> export pipeline as every other series. Series carry yearly
// points whose Date is a bare "YYYY".
package gcat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

const defaultBaseURL = "https://planet4589.org/space/gcat/tsv"

// File paths under the GCAT tsv root. Kept as vars so a real run can correct any
// path mismatch without touching the fetch logic.
var (
	launchPath = "launch/launch.tsv"
	orgsPath   = "tables/orgs.tsv"
	satcatPath = "cat/satcat.tsv"
)

// Synthetic series IDs. These are the wire contract with aggregate.Views.
const (
	idSuccesses = "gcat-orbital-successes"
	idFailures  = "gcat-orbital-failures"

	idUSA    = "gcat-country-usa"
	idChina  = "gcat-country-china"
	idRussia = "gcat-country-russia"
	idOther  = "gcat-country-other"

	idMassUSA    = "gcat-mass-usa"
	idMassChina  = "gcat-mass-china"
	idMassRussia = "gcat-mass-russia"
	idMassOther  = "gcat-mass-other"
)

// Country buckets. USSR ("SU") and modern Russia ("RU") are combined.
const (
	catUSA    = "usa"
	catChina  = "china"
	catRussia = "russia"
	catOther  = "other"
)

// Source fetches and aggregates GCAT data files.
type Source struct {
	baseURL string
	http    *http.Client
}

// New builds a Source. A zero timeout defaults to 120s, since launch.tsv alone
// is >10 MB.
func New(timeout time.Duration) *Source {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Source{
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// Fetch downloads the GCAT files, aggregates them into yearly series, and clips
// each series to [startYear, endYear]. It implements refresh.Source.
func (s *Source) Fetch(ctx context.Context, startYear, endYear int) ([]bls.Series, error) {
	fetchedAt := time.Now().UTC()

	launches, err := s.fetchTable(ctx, launchPath)
	if err != nil {
		return nil, fmt.Errorf("gcat launches: %w", err)
	}
	orgs, err := s.fetchTable(ctx, orgsPath)
	if err != nil {
		return nil, fmt.Errorf("gcat orgs: %w", err)
	}
	sats, err := s.fetchTable(ctx, satcatPath)
	if err != nil {
		return nil, fmt.Errorf("gcat satcat: %w", err)
	}

	var out []bls.Series
	out = append(out, countrySeries(launches, orgState(orgs), fetchedAt)...)
	out = append(out, massCountrySeries(sats, fetchedAt)...)
	out = append(out, outcomeSeries(launches, fetchedAt)...)

	for i := range out {
		out[i].Points = clip(out[i].Points, startYear, endYear)
	}
	return out, nil
}

// fetchTable downloads and parses one GCAT TSV file.
func (s *Source) fetchTable(ctx context.Context, path string) (*table, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return parseTSV(resp.Body)
}

// --- TSV parsing ---

// table is a parsed TSV: column-name -> index plus the data rows. Column names
// are normalized (lowercased, leading '#' and surrounding space stripped) so
// lookups tolerate GCAT's "#Code"-style headers.
type table struct {
	cols map[string]int
	rows [][]string
}

func parseTSV(r io.Reader) (*table, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20) // tolerate long rows

	var header []string
	var rows [][]string
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if header == nil {
			header = strings.Split(line, "\t")
			continue
		}
		// After the header (itself "#"-prefixed), GCAT files carry a "# Updated ..."
		// comment line and sometimes a dashed rule; data rows are never "#"-prefixed.
		if line[0] == '#' || isSeparator(line) {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("empty TSV")
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		cols[normCol(h)] = i
	}
	return &table{cols: cols, rows: rows}, nil
}

func normCol(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")))
}

// isSeparator reports whether a line is only dashes/whitespace/tabs.
func isSeparator(line string) bool {
	for _, r := range line {
		if r != '-' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

// col returns the index of the first matching column name, or -1.
func (t *table) col(names ...string) int {
	for _, n := range names {
		if i, ok := t.cols[normCol(n)]; ok {
			return i
		}
	}
	return -1
}

// field returns the trimmed value at column index i for a row, or "" if out of range.
func field(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// --- Aggregation ---

// vagueYear extracts the 4-digit year from a GCAT VagueDate such as
// "1957 Oct  4 1928:34" (the first whitespace-separated token). Returns 0 if no
// plausible year is found.
func vagueYear(s string) int {
	tok := strings.Fields(s)
	if len(tok) == 0 {
		return 0
	}
	y, err := strconv.Atoi(tok[0])
	if err != nil || y < 1900 || y > 2100 {
		return 0
	}
	return y
}

// outcomeSeries counts successful and failed orbital launches per year.
// LaunchCode's first letter 'O' marks an orbital launch; the second letter is
// S/F/U for Success/Fail/Unknown (partial outcomes like "OS80"/"OF40" still
// start S/F). Both lines share the union of orbital-launch years, with explicit
// zeros so the lines stay continuous.
func outcomeSeries(t *table, fetchedAt time.Time) []bls.Series {
	dateCol := t.col("Launch_Date", "Launch Date")
	codeCol := t.col("LaunchCode", "Launch_Code")

	successes := map[int]float64{}
	failures := map[int]float64{}
	years := map[int]float64{}
	for _, row := range t.rows {
		code := field(row, codeCol)
		if len(code) == 0 || code[0] != 'O' {
			continue
		}
		y := vagueYear(field(row, dateCol))
		if y == 0 {
			continue
		}
		years[y] = 0
		if len(code) >= 2 {
			switch code[1] {
			case 'S':
				successes[y]++
			case 'F':
				failures[y]++
			}
		}
	}

	yrs := sortedKeys(years)
	return []bls.Series{
		series(idSuccesses, "Successes", "launches", yrs, successes, fetchedAt),
		series(idFailures, "Failures", "launches", yrs, failures, fetchedAt),
	}
}

// orgState maps a GCAT organization code to a country category via the orgs table.
func orgState(t *table) map[string]string {
	codeCol := t.col("Code", "Org", "UCode")
	stateCol := t.col("StateCode", "State")
	m := make(map[string]string, len(t.rows))
	for _, row := range t.rows {
		code := field(row, codeCol)
		if code == "" {
			continue
		}
		m[code] = stateCategory(field(row, stateCol))
	}
	return m
}

// stateCategory groups a GCAT StateCode into one of the four country buckets.
func stateCategory(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "US", "USA":
		return catUSA
	case "CN", "PRC":
		return catChina
	case "SU", "RU", "CIS":
		return catRussia
	default:
		return catOther
	}
}

// countrySeries counts orbital launches per year, grouped into the four country
// lines by the launching agency's state. Every country gets an explicit 0 for
// years that had any launch, so the lines stay continuous rather than breaking.
func countrySeries(t *table, agencyCategory map[string]string, fetchedAt time.Time) []bls.Series {
	dateCol := t.col("Launch_Date", "Launch Date")
	codeCol := t.col("LaunchCode", "Launch_Code")
	agencyCol := t.col("Launch_Agency", "Agency", "LV_Manufacturer")

	byCat := map[string]map[int]float64{
		catUSA: {}, catChina: {}, catRussia: {}, catOther: {},
	}
	allYears := map[int]float64{}
	for _, row := range t.rows {
		code := field(row, codeCol)
		if len(code) == 0 || code[0] != 'O' {
			continue
		}
		y := vagueYear(field(row, dateCol))
		if y == 0 {
			continue
		}
		cat, ok := agencyCategory[field(row, agencyCol)]
		if !ok {
			cat = catOther
		}
		byCat[cat][y]++
		allYears[y] = 0
	}

	years := sortedKeys(allYears)
	return []bls.Series{
		series(idUSA, "USA", "launches", years, byCat[catUSA], fetchedAt),
		series(idChina, "China", "launches", years, byCat[catChina], fetchedAt),
		series(idRussia, "Russia / USSR", "launches", years, byCat[catRussia], fetchedAt),
		series(idOther, "Other", "launches", years, byCat[catOther], fetchedAt),
	}
}

// massCountrySeries sums cataloged payload mass reaching orbit per year (tonnes),
// grouped into the four country lines by the satellite's owner state. A satcat
// row is a payload when its Type begins with 'P'; presence in the catalog
// implies it reached orbit. Mass is in kg; rows with no parseable mass are skipped.
func massCountrySeries(t *table, fetchedAt time.Time) []bls.Series {
	dateCol := t.col("LDate", "Launch_Date")
	typeCol := t.col("Type")
	stateCol := t.col("State")
	massCol := t.col("Mass")

	byCat := map[string]map[int]float64{
		catUSA: {}, catChina: {}, catRussia: {}, catOther: {},
	}
	allYears := map[int]float64{}
	for _, row := range t.rows {
		typ := field(row, typeCol)
		if typ == "" || typ[0] != 'P' {
			continue
		}
		kg, err := strconv.ParseFloat(field(row, massCol), 64)
		if err != nil {
			continue
		}
		y := vagueYear(field(row, dateCol))
		if y == 0 {
			continue
		}
		byCat[stateCategory(field(row, stateCol))][y] += kg / 1000.0
		allYears[y] = 0
	}

	years := sortedKeys(allYears)
	return []bls.Series{
		series(idMassUSA, "USA", "tonnes", years, byCat[catUSA], fetchedAt),
		series(idMassChina, "China", "tonnes", years, byCat[catChina], fetchedAt),
		series(idMassRussia, "Russia / USSR", "tonnes", years, byCat[catRussia], fetchedAt),
		series(idMassOther, "Other", "tonnes", years, byCat[catOther], fetchedAt),
	}
}

// --- helpers ---

// series builds a yearly bls.Series over the given sorted years, taking each
// value from byYear (defaulting to 0 so lines stay continuous).
func series(id, label, units string, years []int, byYear map[int]float64, fetchedAt time.Time) bls.Series {
	pts := make([]bls.Point, 0, len(years))
	for _, y := range years {
		v := byYear[y]
		pts = append(pts, bls.Point{Date: strconv.Itoa(y), Value: &v})
	}
	return bls.Series{
		ID:        id,
		Label:     label,
		Units:     units,
		Points:    pts,
		FetchedAt: fetchedAt,
	}
}

func sortedKeys(m map[int]float64) []int {
	years := make([]int, 0, len(m))
	for y := range m {
		years = append(years, y)
	}
	sort.Ints(years)
	return years
}

// clip drops points whose year is outside [startYear, endYear] (open when 0).
func clip(pts []bls.Point, startYear, endYear int) []bls.Point {
	out := pts[:0]
	for _, p := range pts {
		y, err := strconv.Atoi(p.Date)
		if err != nil {
			continue
		}
		if startYear != 0 && y < startYear {
			continue
		}
		if endYear != 0 && y > endYear {
			continue
		}
		out = append(out, p)
	}
	return out
}
