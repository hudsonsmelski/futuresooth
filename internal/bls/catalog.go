package bls

// SeriesMeta is human-facing metadata for a BLS series ID.
type SeriesMeta struct {
	ID                 string
	Label              string
	Description        string
	Units              string
	SeasonallyAdjusted bool
}

// Catalog is the registry of BLS series this service knows about. Series IDs are
// verified against the live BLS API. Add more here to expand coverage (race,
// age, education, U-6, participation, state-level, etc.).
//
// Series ID anatomy (LNS = Labor force statistics, Seasonally adjusted):
//
//	LNS14000000  Unemployment Rate, 16 yrs & over
//	LNS14000001  ... Men, 16 yrs & over
//	LNS14000002  ... Women, 16 yrs & over
var Catalog = map[string]SeriesMeta{
	"LNS14000000": {
		ID:                 "LNS14000000",
		Label:              "All, 16+",
		Description:        "Unemployment rate, 16 years and over",
		Units:              "percent",
		SeasonallyAdjusted: true,
	},
	"LNS14000001": {
		ID:                 "LNS14000001",
		Label:              "Men, 16+",
		Description:        "Unemployment rate, men, 16 years and over",
		Units:              "percent",
		SeasonallyAdjusted: true,
	},
	"LNS14000002": {
		ID:                 "LNS14000002",
		Label:              "Women, 16+",
		Description:        "Unemployment rate, women, 16 years and over",
		Units:              "percent",
		SeasonallyAdjusted: true,
	},

	// By sex, 20 years and over.
	"LNS14000025": {ID: "LNS14000025", Label: "Men, 20+", Description: "Unemployment rate, men, 20 years and over", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000026": {ID: "LNS14000026", Label: "Women, 20+", Description: "Unemployment rate, women, 20 years and over", Units: "percent", SeasonallyAdjusted: true},

	// By race / ethnicity, 16 years and over. (These series begin later than the
	// 1948 overall series: White 1954, Black 1972, Hispanic 1973, Asian 2000.)
	"LNS14000003": {ID: "LNS14000003", Label: "White", Description: "Unemployment rate, White, 16 years and over", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000006": {ID: "LNS14000006", Label: "Black or African American", Description: "Unemployment rate, Black or African American, 16 years and over", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000009": {ID: "LNS14000009", Label: "Hispanic or Latino", Description: "Unemployment rate, Hispanic or Latino, 16 years and over", Units: "percent", SeasonallyAdjusted: true},
	"LNS14032183": {ID: "LNS14032183", Label: "Asian", Description: "Unemployment rate, Asian, 16 years and over", Units: "percent", SeasonallyAdjusted: true},

	// By age group.
	"LNS14000012": {ID: "LNS14000012", Label: "16–19 yrs", Description: "Unemployment rate, 16 to 19 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14024887": {ID: "LNS14024887", Label: "16–24 yrs", Description: "Unemployment rate, 16 to 24 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000036": {ID: "LNS14000036", Label: "20–24 yrs", Description: "Unemployment rate, 20 to 24 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000089": {ID: "LNS14000089", Label: "25–34 yrs", Description: "Unemployment rate, 25 to 34 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000091": {ID: "LNS14000091", Label: "35–44 yrs", Description: "Unemployment rate, 35 to 44 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000093": {ID: "LNS14000093", Label: "45–54 yrs", Description: "Unemployment rate, 45 to 54 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000060": {ID: "LNS14000060", Label: "25–54 yrs", Description: "Unemployment rate, 25 to 54 years", Units: "percent", SeasonallyAdjusted: true},
	"LNS14000048": {ID: "LNS14000048", Label: "25+ yrs", Description: "Unemployment rate, 25 years and over", Units: "percent", SeasonallyAdjusted: true},
	"LNS14024230": {ID: "LNS14024230", Label: "55+ yrs", Description: "Unemployment rate, 55 years and over", Units: "percent", SeasonallyAdjusted: true},
}

// CatalogIDs returns all known series IDs (order is unspecified).
func CatalogIDs() []string {
	ids := make([]string, 0, len(Catalog))
	for id := range Catalog {
		ids = append(ids, id)
	}
	return ids
}

// Lookup returns metadata for a series ID, falling back to a minimal record
// (using the ID as the label) when the series is unknown.
func Lookup(id string) SeriesMeta {
	if m, ok := Catalog[id]; ok {
		return m
	}
	return SeriesMeta{ID: id, Label: id, Units: "percent"}
}
