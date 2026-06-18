// Renders every chart registered by the server in window.__CHARTS__ (keyed by the
// chart container's DOM id), using Observable Plot (the global `Plot`). Each wide
// payload (shared month axis + per-series value arrays) is reshaped into tidy long
// rows, keeping nulls so Plot breaks each line at gaps rather than drawing through.
(function () {
  if (typeof Plot === "undefined") return;
  const registry = window.__CHARTS__ || {};
  const entries = Object.entries(registry);
  if (entries.length === 0) return;

  const parseMonth = (ym) => {
    const [y, m] = ym.split("-").map(Number);
    return new Date(y, m - 1, 1);
  };

  function buildRows(chart) {
    const months = chart.x.values.map(parseMonth);
    const rows = [];
    for (const s of chart.series) {
      for (let i = 0; i < months.length; i++) {
        rows.push({ date: months[i], series: s.label, value: s.values[i] });
      }
    }
    return rows;
  }

  function renderOne(el, chart) {
    const multi = chart.series.length > 1;
    const yLabel = chart.units === "percent" ? "% unemployed" : chart.units;
    const rows = buildRows(chart);
    const width = el.clientWidth || 640;
    const fig = Plot.plot({
      width,
      height: Math.max(260, Math.round(width * 0.55)),
      marginLeft: 48,
      marginBottom: 32,
      x: { type: "time", label: null, grid: true },
      y: { label: yLabel, grid: true, zero: true },
      color: { legend: multi },
      marks: [
        Plot.ruleY([0]),
        Plot.lineY(rows, {
          x: "date",
          y: "value",
          stroke: multi ? "series" : undefined,
          curve: "monotone-x",
          strokeWidth: 1.75,
        }),
        Plot.tip(rows, Plot.pointerX({ x: "date", y: "value", stroke: multi ? "series" : undefined })),
      ],
    });
    el.replaceChildren(fig);
  }

  function renderAll() {
    for (const [id, chart] of entries) {
      const el = document.getElementById(id);
      if (el) renderOne(el, chart);
    }
  }

  renderAll();

  let raf;
  window.addEventListener("resize", () => {
    cancelAnimationFrame(raf);
    raf = requestAnimationFrame(renderAll);
  });
})();
