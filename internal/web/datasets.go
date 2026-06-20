package web

// Dataset groups related backend views under one selectable card on the home
// page. For now there's a single dataset (unemployment); more will be added as
// the backend grows. A dataset claims every backend view whose key starts with
// KeyPrefix.
type Dataset struct {
	Slug      string
	Title     string
	Subtitle  string
	KeyPrefix string
}

var datasets = []Dataset{
	{
		Slug:      "unemployment",
		Title:     "Unemployment Rate",
		Subtitle:  "U.S. unemployment rate from the Bureau of Labor Statistics",
		KeyPrefix: "unemployment-",
	},
	{
		Slug:      "space",
		Title:     "Space Industry",
		Subtitle:  "Orbital launch activity from GCAT (J. McDowell)",
		KeyPrefix: "space-",
	},
	{
		Slug:      "inflation",
		Title:     "Inflation",
		Subtitle:  "U.S. consumer prices by category (BLS CPI)",
		KeyPrefix: "inflation-",
	},
	{
		Slug:      "population",
		Title:     "Population",
		Subtitle:  "U.S. population by age, race, and Hispanic origin (Census Bureau)",
		KeyPrefix: "population-",
	},
}

// datasetBySlug returns the dataset with the given slug, if it exists.
func datasetBySlug(slug string) (Dataset, bool) {
	for _, d := range datasets {
		if d.Slug == slug {
			return d, true
		}
	}
	return Dataset{}, false
}
