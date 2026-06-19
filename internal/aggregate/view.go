// Package aggregate defines curated "views" (a titled set of BLS series) and
// merges separate series pulls onto one shared time axis to produce chart-ready
// data for a client to render.
package aggregate

import "strings"

// View is a curated chart definition: a human title plus the set of BLS series
// IDs whose lines belong on the same plot.
type View struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle,omitempty"`
	Units     string   `json:"units"`
	SeriesIDs []string `json:"series_ids"`
	// Slug is the descriptive base filename for this view's on-disk data files
	// (e.g. "unemployment_rate_by_sex" -> .json and .csv). Falls back to the key
	// with dashes turned into underscores when empty.
	Slug string `json:"slug,omitempty"`
	// Source overrides the default attribution for this view (empty = BLS default).
	Source string `json:"source,omitempty"`
	// Annual marks a view whose axis steps are years ("YYYY") rather than months.
	Annual bool `json:"annual,omitempty"`
	// Colors optionally sets per-line CSS colors, positionally aligned to
	// SeriesIDs. Empty entries (or a short/absent slice) fall back to the
	// default color scheme.
	Colors []string `json:"colors,omitempty"`
}

// gcatCitation is the CC-BY attribution required for GCAT-sourced views.
const gcatCitation = "GCAT (J. McDowell, planet4589.org/space/gcat)"

// FileBase returns the base filename (no extension) for this view's data files.
func (v View) FileBase() string {
	if v.Slug != "" {
		return v.Slug
	}
	return strings.ReplaceAll(v.Key, "-", "_")
}

// Views is the registry of curated charts. This is the "smart curated set of
// plots" surface; add entries here as the catalog grows.
var Views = []View{
	{
		Key:       "unemployment-overall",
		Title:     "Unemployment Rate",
		Subtitle:  "16 years and over, seasonally adjusted",
		Units:     "percent",
		SeriesIDs: []string{"LNS14000000"},
		Slug:      "unemployment_rate_overall",
	},
	{
		Key:       "unemployment-by-sex",
		Title:     "Unemployment Rate by Sex",
		Subtitle:  "16 years and over, seasonally adjusted",
		Units:     "percent",
		SeriesIDs: []string{"LNS14000001", "LNS14000002"},
		Slug:      "unemployment_rate_by_sex",
	},
	{
		Key:       "unemployment-by-race",
		Title:     "Unemployment Rate by Race & Ethnicity",
		Subtitle:  "16 years and over, seasonally adjusted",
		Units:     "percent",
		SeriesIDs: []string{"LNS14000003", "LNS14000006", "LNS14000009", "LNS14032183"},
		Slug:      "unemployment_rate_by_race",
	},
	{
		Key:       "unemployment-by-age",
		Title:     "Unemployment Rate by Age",
		Subtitle:  "Seasonally adjusted",
		Units:     "percent",
		SeriesIDs: []string{"LNS14024887", "LNS14000060", "LNS14024230"},
		Slug:      "unemployment_rate_by_age",
	},

	// --- Space industry (GCAT). Yearly series; IDs match internal/gcat output. ---
	{
		Key:       "space-by-country",
		Title:     "Orbital Launches by Country",
		Subtitle:  "Orbital launch attempts per year, by launching state",
		Units:     "launches",
		SeriesIDs: []string{"gcat-country-usa", "gcat-country-china", "gcat-country-russia", "gcat-country-other"},
		Slug:      "orbital_launches_by_country",
		Source:    gcatCitation,
		Annual:    true,
	},
	{
		Key:       "space-mass",
		Title:     "Mass to Orbit by Country",
		Subtitle:  "Cataloged payload mass reaching orbit per year, by owner state",
		Units:     "tonnes",
		SeriesIDs: []string{"gcat-mass-usa", "gcat-mass-china", "gcat-mass-russia", "gcat-mass-other"},
		Slug:      "mass_to_orbit_by_country",
		Source:    gcatCitation,
		Annual:    true,
	},
	{
		Key:       "space-outcomes",
		Title:     "Orbital Launch Outcomes",
		Subtitle:  "Successful vs. failed orbital launches per year",
		Units:     "launches",
		SeriesIDs: []string{"gcat-orbital-successes", "gcat-orbital-failures"},
		Colors:    []string{"#2ca02c", "#d62728"}, // success green, failure red
		Slug:      "orbital_launch_outcomes",
		Source:    gcatCitation,
		Annual:    true,
	},
}

// ViewByKey returns the view with the given key, if it exists.
func ViewByKey(key string) (View, bool) {
	for _, v := range Views {
		if v.Key == key {
			return v, true
		}
	}
	return View{}, false
}

// AllSeriesIDs returns the de-duplicated set of every series ID referenced by
// any view. Useful for deciding what to fetch/refresh.
func AllSeriesIDs() []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, v := range Views {
		for _, id := range v.SeriesIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids
}
