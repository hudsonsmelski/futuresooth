package web

import (
	"embed"
	"html/template"
)

//go:embed templates
var templatesFS embed.FS

// Page template sets. Each page file defines "title" and "content"; base.html is
// the skeleton that invokes them and is the template executed by name.
var (
	indexTmpl   = mustParse("templates/base.html", "templates/index.html")
	datasetTmpl = mustParse("templates/base.html", "templates/dataset.html", "templates/chart_section.html")
	viewTmpl    = mustParse("templates/base.html", "templates/view.html", "templates/chart_section.html")
	errorTmpl   = mustParse("templates/base.html", "templates/error.html")
)

func mustParse(files ...string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, files...))
}
