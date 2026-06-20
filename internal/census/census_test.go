package census

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

// a PEP charv response: header + total/single-year rows (ignored) + the 5-year
// bands we keep, for both sexes.
var pepRows = [][]string{
	{"POP", "AGE", "SEX", "YEAR", "POPGROUP", "HISP", "us"},
	{"334914895", "0000", "0", "2023", "001", "0", "1"}, // all ages -> ignored
	{"3694222", "0100", "0", "2023", "001", "0", "1"},   // single year -> ignored
	{"9000000", "0401", "1", "2023", "001", "0", "1"},   // 0-4 male
	{"8500000", "0401", "2", "2023", "001", "0", "1"},   // 0-4 female
	{"6000000", "8599", "1", "2023", "001", "0", "1"},   // 85+ male
	{"9000000", "8599", "2", "2023", "001", "0", "1"},   // 85+ female
}

func TestPyramidSeries(t *testing.T) {
	got := pyramidSeries(pepRows, time.Now())
	if len(got) != 2 {
		t.Fatalf("got %d series, want 2", len(got))
	}
	male := byBand(got, idPyramidMale)
	female := byBand(got, idPyramidFemale)

	// Every band is present (0-filled) and in canonical order.
	if len(male) != len(ageBands) {
		t.Errorf("male has %d bands, want %d", len(male), len(ageBands))
	}
	if male["00-04"] != 9_000_000 || female["00-04"] != 8_500_000 {
		t.Errorf("0-4 m/f = %v/%v", male["00-04"], female["00-04"])
	}
	if male["85+"] != 6_000_000 || female["85+"] != 9_000_000 {
		t.Errorf("85+ m/f = %v/%v", male["85+"], female["85+"])
	}
	if male["10-14"] != 0 { // not in fixture -> 0-filled, not missing
		t.Errorf("10-14 male = %v, want 0", male["10-14"])
	}
	// First point is the youngest band.
	if got[0].Points[0].Date != "00-04" {
		t.Errorf("first band = %q, want 00-04", got[0].Points[0].Date)
	}
}

func TestFetchACSNativesAndSkip(t *testing.T) {
	// ACS rows for one year; Natives = AIAN(_004) + NHPI(_006).
	acs := func(white, black, aian, asian, nhpi, multi, notHisp, hisp string) string {
		b, _ := json.Marshal([][]string{
			{"B02001_002E", "B02001_003E", "B02001_004E", "B02001_005E", "B02001_006E", "B02001_008E", "B03003_002E", "B03003_003E", "us"},
			{white, black, aian, asian, nhpi, multi, notHisp, hisp, "1"},
		})
		return string(b)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/2023/acs/acs1", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(acs("200", "40", "3", "20", "1", "42", "270", "65")))
	})
	// 2024 returns 404 -> must be skipped, not fatal.
	mux.HandleFunc("/2024/acs/acs1", func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := New("testkey", 0, nil)
	s.baseURL = srv.URL

	out, err := s.fetchACS(context.Background(), 2023, 2024, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	natives := byYear(out, idRaceNative)
	if natives["2023"] != 4 { // 3 (AIAN) + 1 (NHPI)
		t.Errorf("2023 Natives = %v, want 4", natives["2023"])
	}
	if white := byYear(out, idRaceWhite); white["2023"] != 200 {
		t.Errorf("2023 White = %v, want 200", white["2023"])
	}
	if hisp := byYear(out, idHispYes); hisp["2023"] != 65 {
		t.Errorf("2023 Hispanic = %v, want 65", hisp["2023"])
	}
	if _, has2024 := byYear(out, idRaceWhite)["2024"]; has2024 {
		t.Errorf("2024 should have been skipped (404)")
	}
}

func TestFetchACSReusesCache(t *testing.T) {
	// existing reports every race/hisp series already cached for 2023.
	existing := func(id string) (bls.Series, bool) {
		return bls.Series{ID: id, Points: []bls.Point{{Date: "2023", Value: ptrf(7)}}}, true
	}
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { called = true; http.NotFound(w, nil) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := New("testkey", 0, existing)
	s.baseURL = srv.URL

	out, err := s.fetchACS(context.Background(), 2023, 2023, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Errorf("API was queried for a year already cached")
	}
	if byYear(out, idRaceWhite)["2023"] != 7 {
		t.Errorf("cached 2023 value not reused")
	}
}

func ptrf(f float64) *float64 { return &f }

func TestFetchEndToEnd(t *testing.T) {
	pep, _ := json.Marshal(pepRows)
	mux := http.NewServeMux()
	mux.HandleFunc("/2023/pep/charv", func(w http.ResponseWriter, _ *http.Request) { w.Write(pep) })
	mux.HandleFunc("/2023/acs/acs1", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal([][]string{
			{"B02001_002E", "B02001_003E", "B02001_004E", "B02001_005E", "B02001_006E", "B02001_008E", "B03003_002E", "B03003_003E", "us"},
			{"200", "40", "3", "20", "1", "42", "270", "65", "1"},
		})
		w.Write(b)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := New("testkey", 0, nil)
	s.baseURL = srv.URL

	out, err := s.Fetch(context.Background(), 2023, 2023)
	if err != nil {
		t.Fatal(err)
	}
	// 2 pyramid + 5 race + 2 hispanic = 9 series.
	if len(out) != 9 {
		t.Errorf("got %d series, want 9", len(out))
	}
}

func TestFetchNoKey(t *testing.T) {
	out, err := New("", 0, nil).Fetch(context.Background(), 2020, 2024)
	if err != nil || out != nil {
		t.Errorf("no-key Fetch = (%v, %v), want (nil, nil)", out, err)
	}
}

func byBand(series []bls.Series, id string) map[string]float64 { return flatten(series, id) }
func byYear(series []bls.Series, id string) map[string]float64 { return flatten(series, id) }

func flatten(series []bls.Series, id string) map[string]float64 {
	for _, s := range series {
		if s.ID == id {
			m := make(map[string]float64, len(s.Points))
			for _, p := range s.Points {
				if p.Value != nil {
					m[p.Date] = *p.Value
				}
			}
			return m
		}
	}
	return nil
}
