// kult_kharis.js — pure formatting for the kharis NET figure and the
// devotion-idle warning shown in the Kult drawer. No DOM, no fetch: kult.js
// already fetches `pd` (the province GET's settlement object) and hands it
// here; this module only decides what HTML (if any) to draw from fields
// that are already present in that payload.
//
// Invariant (CLAUDE.md contract, kharis net surfacing, 2026-07-31): the
// client NEVER computes the kharis net itself — the number comes from the
// server or is not shown. `kharis_net_known === false` means "this Wanax
// has no temples" — draw no net row at all, not even "+0.0".
//
// Kept import-free on purpose: kult.js also imports ../misc.js, which runs
// `document.addEventListener(...)` at module top level, so importing kult.js
// itself crashes under node --test. This module must stay free of any import
// that touches document/window at load time so it can be unit-tested in
// isolation (see kult_kharis.test.mjs).

export function kharisNetView(pd) {
  const out = { netHtml: '', idleHtml: '' };
  if (!pd) return out;

  // Distinct from the existing "Passive" row (pd.kharis_per_day, rendered by
  // kult.js) — that is the geographic rate; this is the daily maintenance
  // net (temple gain − decay), the figure that actually answers "did raising
  // devotion help". Same format as the passive row: sign + one decimal.
  if (pd.kharis_net_known === true) {
    const v = pd.kharis_net_per_day || 0;
    out.netHtml = '<div class="stat-row"><span class="sr-label">Net</span><span class="sr-val">' +
      (v >= 0 ? '+' : '') + v.toFixed(1) + ' kharis/day</span></div>';
  }

  if (pd.kharis_devotion_idle === true) {
    out.idleHtml = '<p class="kult-warn">Temple capacity is standing idle — raise cult labor to put it to work.</p>';
  }

  return out;
}
