package gcat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hudsonsmelski/futuresooth/internal/bls"
)

func TestVagueYear(t *testing.T) {
	cases := map[string]int{
		"1957 Oct  4 1928:34": 1957,
		"2024 May 30":         2024,
		"1969 Jul 20 2017:40": 1969,
		"":                    0,
		"garbage":             0,
		"3500 Jan 1":          0, // out of range
	}
	for in, want := range cases {
		if got := vagueYear(in); got != want {
			t.Errorf("vagueYear(%q) = %d, want %d", in, got, want)
		}
	}
}

// header + dashed separator + rows, the GCAT TSV shape.
const launchTSV = "#Launch_Tag\tLaunch_Date\tLV_Type\tLaunchCode\tLaunch_Agency\n" +
	"--------\t-----------\t-------\t----------\t-------------\n" +
	"1957-001\t1957 Oct  4 1928:34\tSputnik\tOS\tOKB1\n" +
	"1957-F01\t1957 Dec  6 1644\tVanguard\tOF\tUSN\n" +
	"2024-100\t2024 Jan 3 0000\tFalcon 9\tOS\tSPX\n" +
	"2024-F02\t2024 Feb 1 0000\tStarship\tOF\tSPX\n" +
	"S1\t2024 Mar 1\tNotOrbital\tM\tUSAF\n" // suborbital, must be ignored

const orgsTSV = "#Code\tStateCode\tName\n" +
	"-----\t---------\t----\n" +
	"OKB1\tSU\tKorolev\n" +
	"USN\tUS\tUS Navy\n" +
	"SPX\tUS\tSpaceX\n"

const satcatTSV = "#JCAT\tLDate\tType\tState\tMass\n" +
	"-----\t-----\t----\t-----\t----\n" +
	"S00001\t1957 Oct  4\tP\tSU\t83.6\n" +
	"S00002\t2024 Jan 3\tP\tUS\t1000\n" +
	"S00003\t2024 Jan 3\tR\tUS\t2000\n" + // rocket body, not a payload
	"S00004\t2024 Feb 1\tP\tUS\t\n" + // payload with unknown mass, skipped
	"S00005\t2024 Mar 1\tP\tCN\t500\n" // China payload

func TestOutcomeSeries(t *testing.T) {
	tbl, err := parseTSV(strings.NewReader(launchTSV))
	if err != nil {
		t.Fatal(err)
	}
	got := outcomeSeries(tbl, time.Now())
	successes := byDate(got, idSuccesses)
	failures := byDate(got, idFailures)

	if successes["1957"] != 1 || failures["1957"] != 1 { // Sputnik (OS) + Vanguard (OF)
		t.Errorf("1957 successes/failures = %v/%v, want 1/1", successes["1957"], failures["1957"])
	}
	if successes["2024"] != 1 || failures["2024"] != 1 {
		t.Errorf("2024 successes/failures = %v/%v, want 1/1", successes["2024"], failures["2024"])
	}
}

func TestCountrySeries(t *testing.T) {
	lt, _ := parseTSV(strings.NewReader(launchTSV))
	ot, _ := parseTSV(strings.NewReader(orgsTSV))
	got := countrySeries(lt, orgState(ot), time.Now())

	usa := byDate(got, idUSA)
	russia := byDate(got, idRussia)
	if russia["1957"] != 1 { // OKB1 -> SU
		t.Errorf("1957 russia = %v, want 1", russia["1957"])
	}
	if usa["1957"] != 1 { // USN -> US
		t.Errorf("1957 usa = %v, want 1", usa["1957"])
	}
	if usa["2024"] != 2 { // both SPX launches -> US
		t.Errorf("2024 usa = %v, want 2", usa["2024"])
	}
}

func TestMassCountrySeries(t *testing.T) {
	st, _ := parseTSV(strings.NewReader(satcatTSV))
	got := massCountrySeries(st, time.Now())
	usa := byDate(got, idMassUSA)
	china := byDate(got, idMassChina)
	russia := byDate(got, idMassRussia)

	if usa["2024"] != 1.0 { // 1000 kg = 1 tonne; rocket body + unknown-mass payload excluded
		t.Errorf("2024 USA mass = %v tonnes, want 1.0", usa["2024"])
	}
	if china["2024"] != 0.5 { // 500 kg
		t.Errorf("2024 China mass = %v tonnes, want 0.5", china["2024"])
	}
	if russia["1957"] != 0.0836 { // 83.6 kg, SU
		t.Errorf("1957 Russia mass = %v tonnes, want 0.0836", russia["1957"])
	}
}

func TestFetchEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/"+launchPath, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(launchTSV)) })
	mux.HandleFunc("/"+orgsPath, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(orgsTSV)) })
	mux.HandleFunc("/"+satcatPath, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(satcatTSV)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := New(0)
	s.baseURL = srv.URL

	out, err := s.Fetch(context.Background(), 2000, 2024)
	if err != nil {
		t.Fatal(err)
	}
	successes := byDate(out, idSuccesses)
	if _, has1957 := successes["1957"]; has1957 {
		t.Errorf("1957 should be clipped out by startYear=2000")
	}
	if successes["2024"] != 1 {
		t.Errorf("2024 successes = %v, want 1", successes["2024"])
	}
	// 2 outcome + 4 country + 4 mass-by-country series.
	if len(out) != 10 {
		t.Errorf("got %d series, want 10", len(out))
	}
}

// byDate flattens the named series' points into date->value for assertions.
func byDate(series []bls.Series, id string) map[string]float64 {
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
