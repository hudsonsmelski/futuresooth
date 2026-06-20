// Renders every chart registered by the server in window.__CHARTS__ (keyed by the
// chart container's DOM id), using Observable Plot (the global `Plot`). Each wide
// payload (shared time axis + per-series value arrays) is reshaped into tidy long
// rows, keeping nulls so Plot breaks each line at gaps rather than drawing through.
(function () {
  if (typeof Plot === "undefined") return;
  const registry = window.__CHARTS__ || {};
  const entries = Object.entries(registry);
  if (entries.length === 0) return;

  // Axis values are either "YYYY-MM" (monthly) or a bare "YYYY" (yearly).
  const parseAxisDate = (v) => {
    const [y, m] = v.split("-").map(Number);
    return new Date(y, (m || 1) - 1, 1);
  };

  function buildRows(chart) {
    const dates = chart.x.values.map(parseAxisDate);
    const rows = [];
    for (const s of chart.series) {
      for (let i = 0; i < dates.length; i++) {
        rows.push({ date: dates[i], series: s.label, value: s.values[i] });
      }
    }
    return rows;
  }

  // compact formats large numbers for axes/labels: millions and billions are
  // abbreviated (e.g. 203000000 -> "203M"), smaller numbers keep separators.
  const compact = (n) => {
    const a = Math.abs(n);
    const trim = (x) => x.toFixed(1).replace(/\.0$/, "");
    if (a >= 1e9) return trim(n / 1e9) + "B";
    if (a >= 1e6) return trim(n / 1e6) + "M";
    return n.toLocaleString();
  };

  // colorScale builds an explicit color mapping when every series carries a
  // color hint; otherwise it falls back to Plot's default scheme.
  function colorScale(chart, multi) {
    const opt = { legend: multi };
    if (multi && chart.series.every((s) => s.color)) {
      opt.domain = chart.series.map((s) => s.label);
      opt.range = chart.series.map((s) => s.color);
    }
    return opt;
  }

  // renderPyramid draws a back-to-back population pyramid: x-values are age
  // bands, series are Male (drawn left/negative) and Female (right/positive).
  function renderPyramid(el, chart) {
    const bands = chart.x.values;
    const rows = [];
    for (const s of chart.series) {
      const sign = s.label.toLowerCase() === "male" ? -1 : 1;
      for (let i = 0; i < bands.length; i++) {
        const v = s.values[i];
        rows.push({ age: bands[i], series: s.label, pop: v == null ? null : sign * v });
      }
    }
    const width = el.clientWidth || 640;
    const male = rows.filter((r) => r.pop != null && r.pop < 0);
    const female = rows.filter((r) => r.pop != null && r.pop > 0);
    const fig = Plot.plot({
      width,
      height: Math.max(380, bands.length * 22),
      marginLeft: 52,
      marginRight: 44,
      x: { label: chart.units || "Population", tickFormat: (d) => compact(Math.abs(d)), grid: true },
      y: { domain: [...bands].reverse(), label: "Age" }, // oldest at top
      color: colorScale(chart, true),
      marks: [
        Plot.barX(rows, { y: "age", x: "pop", fill: "series" }),
        // Count labels just past each bar's tip so values are readable at a glance.
        Plot.text(male, { y: "age", x: "pop", text: (d) => compact(-d.pop), textAnchor: "end", dx: 30, fontSize: 10 }),
        Plot.text(female, { y: "age", x: "pop", text: (d) => compact(d.pop), textAnchor: "start", dx: -30, fontSize: 10 }),
        Plot.ruleX([0]),
        Plot.tip(rows, Plot.pointer({ y: "age", x: "pop", fill: "series" })),
      ],
    });
    el.replaceChildren(fig);
  }

  function renderOne(el, chart) {
    if (chart.chart === "pyramid") {
      renderPyramid(el, chart);
      return;
    }
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
      y: { label: yLabel, grid: true, zero: true, tickFormat: compact },
      color: colorScale(chart, multi),
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
