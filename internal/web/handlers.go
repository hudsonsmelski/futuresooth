package web

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/hudsonsmelski/futuresooth/internal/aggregate"
	"github.com/hudsonsmelski/futuresooth/internal/apiclient"
)

// Handlers bundles the dependencies the HTTP handlers require.
type Handlers struct {
	client *apiclient.Client
}

// NewHandlers constructs Handlers backed by the given API client.
func NewHandlers(client *apiclient.Client) *Handlers {
	return &Handlers{client: client}
}

// chartVM is the view-model for one rendered chart: its data plus a DOM id and
// the JSON the page hands to Observable Plot.
type chartVM struct {
	Chart aggregate.ChartData
	DomID string
	JSON  template.JS
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// indexPage lists the available datasets. It needs no backend call, so the home
// page renders even if the backend is down.
func (h *Handlers) indexPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, indexTmpl, map[string]any{"Datasets": datasets})
}

// dataset renders every chart in a dataset as an inline list (no click-through).
func (h *Handlers) dataset(w http.ResponseWriter, r *http.Request) {
	ds, ok := datasetBySlug(r.PathValue("dataset"))
	if !ok {
		h.renderError(w, http.StatusNotFound, "Dataset not found", "There's no dataset with that name.")
		return
	}

	views, err := h.client.ListViews(r.Context())
	if err != nil {
		log.Printf("dataset %q: list views: %v", ds.Slug, err)
		h.renderError(w, http.StatusBadGateway, "Data service unavailable",
			"Couldn't reach the data service. Please try again shortly.")
		return
	}

	var charts []chartVM
	var matched int
	for _, v := range views {
		if !strings.HasPrefix(v.Key, ds.KeyPrefix) {
			continue
		}
		matched++
		vm, err := h.buildChartVM(r.Context(), v.Key)
		if err != nil {
			log.Printf("dataset %q: chart %q: %v", ds.Slug, v.Key, err)
			continue // skip a single failing chart rather than failing the page
		}
		charts = append(charts, vm)
	}

	// If the dataset has charts but none loaded, treat it as a backend problem.
	if matched > 0 && len(charts) == 0 {
		h.renderError(w, http.StatusBadGateway, "Data service unavailable",
			"Couldn't load these charts right now. Please try again shortly.")
		return
	}

	h.render(w, http.StatusOK, datasetTmpl, map[string]any{"Dataset": ds, "Charts": charts})
}

// view renders a single chart (deep-link friendly).
func (h *Handlers) view(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	vm, err := h.buildChartVM(r.Context(), key)
	if err != nil {
		log.Printf("view %q: %v", key, err)
		if strings.Contains(err.Error(), "404") {
			h.renderError(w, http.StatusNotFound, "Chart not found", "There's no chart with that name.")
			return
		}
		h.renderError(w, http.StatusBadGateway, "Data service unavailable",
			"Couldn't load this chart right now. Please try again shortly.")
		return
	}
	h.render(w, http.StatusOK, viewTmpl, map[string]any{"Section": vm})
}

// buildChartVM fetches a view's chart data and prepares it for rendering.
func (h *Handlers) buildChartVM(ctx context.Context, key string) (chartVM, error) {
	chart, err := h.client.GetView(ctx, key)
	if err != nil {
		return chartVM{}, err
	}
	raw, err := json.Marshal(chart)
	if err != nil {
		return chartVM{}, err
	}
	// Emit as a trusted JS value (assigned to a global in the template). template.JS
	// is output verbatim in script context; json.Marshal already \u-escapes <,>,&
	// so it can't break out of the <script> element.
	return chartVM{Chart: chart, DomID: "chart-" + key, JSON: template.JS(raw)}, nil
}

// renderError renders the friendly error page with the given status.
func (h *Handlers) renderError(w http.ResponseWriter, status int, title, msg string) {
	h.render(w, status, errorTmpl, map[string]any{"Title": title, "Message": msg})
}

// render executes a template into a buffer first, so a template error doesn't
// emit a half-written page under the wrong status code.
func (h *Handlers) render(w http.ResponseWriter, status int, tmpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("render: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
