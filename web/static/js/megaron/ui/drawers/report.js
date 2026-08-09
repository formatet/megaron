import { State, ownCapital, activeCitySettlement } from '../../state.js';
import { fetchAuth } from '../../api.js';

// ── Report drawer (B1, megaron_mvp_mandag.md §B1) ─────────────────────────
// Deliberately primitive: kind + free text, sent to POST /reports. Position
// and "what were you looking at" are attached automatically — State.previousDrawer
// (set by main.js's closeDrawer, the one choke point every drawer transition
// passes through) names whichever drawer was open the moment Report was
// opened, and State.selectedHex/ownCapital() supply a hex when nothing was
// explicitly clicked.

// buildContext (mig 123, temenos_buggrapporter.md "Nån smart input till
// buggrapporteringen"): q/r/view say WHERE the player was, not WHICH entity
// they were looking at — tracing a report to its unit/settlement used to mean
// reverse-engineering it from timing and position after the fact. Free-form
// rather than one field per entity kind, since the drawer set keeps growing
// and a fixed schema would need a migration for every new one; this just
// forwards whatever of the panel-local State the currently (or most
// recently) open panel happens to carry. Reading State directly is safe even
// though opening the Report drawer visually covers the march-ctx/inspect
// panels behind it (report.js is a separate drawer, .march-ctx sits at a
// higher z-index than any drawer and .inspect-panel is merely obscured, not
// closed) — nothing here gets cleared just because Report is open on top of it.
export function buildContext() {
  const ctx = {};
  // The march-order menu (ui/marchctx.js) is a floating panel independent of
  // the drawer system — it can be open regardless of which drawer (if any)
  // Report's view names, so it's checked unconditionally.
  if (State.marchCtxDest) {
    ctx.march_ctx_dest = { q: State.marchCtxDest.q, r: State.marchCtxDest.r };
    if (State.marchCtxUnits?.length) {
      ctx.unit_ids = State.marchCtxUnits.map(u => u.id);
    }
  }
  // The City drawer's settlement is only meaningful when City was the drawer
  // Report's `view` already names — attaching it unconditionally would imply
  // a city context for reports that have nothing to do with one.
  if (State.previousDrawer === 'city') {
    const settlement = activeCitySettlement();
    if (settlement) ctx.settlement_id = settlement.id;
  }
  return Object.keys(ctx).length ? ctx : undefined;
}

export async function submitReport() {
  const textEl = document.getElementById('report-text');
  const kindEl = document.getElementById('report-kind');
  const statusEl = document.getElementById('report-status');
  const text = (textEl?.value || '').trim();
  if (!text) {
    if (statusEl) statusEl.textContent = 'Write something first.';
    return;
  }

  const body = { kind: kindEl?.value || 'bug', body: text };
  const hex = State.selectedHex || ownCapital();
  if (hex && hex.q !== undefined && hex.r !== undefined) {
    body.q = hex.q;
    body.r = hex.r;
  }
  if (State.previousDrawer) body.view = State.previousDrawer;
  const context = buildContext();
  if (context) body.context = context;

  if (statusEl) statusEl.textContent = 'Sending…';
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/reports`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      if (statusEl) statusEl.textContent = 'Could not send — try again.';
      return;
    }
    if (textEl) textEl.value = '';
    if (statusEl) statusEl.textContent = 'Sent. Thank you.';
  } catch (_) {
    if (statusEl) statusEl.textContent = 'Could not send — try again.';
  }
}
