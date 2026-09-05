// sitos_view.js — pure derivation of the Sitos-magasinet STATE text, and the
// stat-row that renders it. No DOM, no fetch: city.js already fetches `pd`
// (the province GET's settlement object, `pd.sitos`) and hands the relevant
// fields here.
//
// city.js has shown coverage and granary totals since 76d21666 (2026-07-13)
// and 7ea8d40 (2026-08-03) — that part was never missing. What was missing
// (megaron_plan_sitos_utlosning.md §5, corrected 2026-08-26) is the STATE
// keryx prints alongside those same numbers: `keryx status` derives one of
// five Swedish phrases from coverage vs. the low/high thresholds plus the
// stock and its direction (server/cmd/keryx/cmd_status.go:362-372). No field
// on the wire carries that text yet on master (province.go:830-838 sends the
// raw numbers — granary_total, granary_cap, coverage_ticks, food_net_per_tick,
// low_ticks, high_ticks — but not a precomputed state string; that only
// exists as an unmerged, uncommitted-to-master extraction on branch
// foda/magasinsraden). Until that lands and the client can just read the
// field, this mirrors the same five branches, in the same order, off the
// same five inputs already on the wire — so a drift in either copy is a
// one-function diff, not a design decision.
//
// Kept import-free on purpose (see kult_kharis.js's identical note): city.js
// also imports modules that touch document/window at load time, so this
// stays free of any such import to remain unit-testable in isolation.

export function sitosGranaryState(cov, low, high, total, net) {
  if (cov < low && total <= 0 && net > 0) {
    return { text: 'empty — but the stock is growing, coverage is rising', severity: 'empty-growing' };
  }
  if (cov < low && total <= 0) {
    return { text: 'EMPTY and the stock is shrinking — the city is unprotected', severity: 'empty-shrinking' };
  }
  if (cov < low) {
    return { text: 'releasing food to the city', severity: 'release' };
  }
  if (cov <= high) {
    return { text: 'resting — neither storing nor releasing', severity: 'rest' };
  }
  return { text: 'storing away a tenth of the surplus', severity: 'store' };
}

// sitosStateHtml renders the state as one stat-row, using the same `sitos`
// sub-object city.js already reads (pd.sitos) — never a second fetch. A
// settlement without the sitos object (or with it null/undefined) renders
// nothing rather than guessing zeros, matching the surrounding section's
// existing "no sitos → empty" behaviour.
export function sitosStateHtml(s) {
  if (!s) return '';
  const cov   = s.coverage_ticks    || 0;
  const low   = s.low_ticks         || 0;
  const high  = s.high_ticks        || 0;
  const total = s.granary_total     || 0;
  const net   = s.food_net_per_tick || 0;
  const { text, severity } = sitosGranaryState(cov, low, high, total, net);
  return `<div class="stat-row"><span class="sr-label">State</span>` +
    `<span class="sr-val sitos-state-${severity}">${text}</span></div>`;
}
