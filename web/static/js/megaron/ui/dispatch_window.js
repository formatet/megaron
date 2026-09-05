import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { notifText, notifIcon, colonyFoundedGrainLine } from './format.js';

// centreOn (ui/search.js) is reached via the window.* bridge, not a direct
// import: search.js touches real DOM elements (#search-input) at MODULE TOP
// LEVEL, and importing it here would drag that into every module that needs
// the dispatch window (ui/chips.js, ui/drawers/notif.js) — exactly the
// fragility ws.js's own header comment warns about. Same convention as
// ws.js → window.addDispatch.

// ── Dispatch window — "the bearing form" (megaron_plan_dispatches.md §1) ──
// A click on a dispatch chip and a click on a Notifications archive row open
// exactly this same window — two doors, one room. Inside: the information,
// a button that jumps the map to where it happened, and a mute checkbox for
// "this kind of dispatch shouldn't come any more". The checkbox IS the
// preference (§2/§6): it always reads and writes the server-side truth
// (GET/PUT/DELETE /api/v1/notification-preferences), never a client-only
// toggle that would forget itself on reload.
//
// Muting only ever affects the transient chip — the event is always in the
// Notifications archive regardless (see notify.Hub.NotifyPlayer server-side).

// The three trade-offer resolution notices carry BOTH the recipient's own
// settlement_id AND the counterparty's (counterparty_id) — Timothy
// 2026-09-04 (megaron_plan_dispatches.md §3): "the button goes to the
// counterparty's city, that's the one the player doesn't already see."
const COUNTERPARTY_KINDS = new Set(['OfferAccepted', 'OfferDeclined', 'OfferExpired']);

// resolveDestination turns a notification payload into a {q,r} hex, honouring
// every shape the ~30 NotifyPlayer kinds actually use (megaron_plan_
// dispatches.md §6:3 audit): a direct hex, a settlement id under one of its
// several field names, or — as a last resort — a unit id looked up in
// currently-loaded map data. Returns null only for a payload that truly
// carries no destination (should not happen post-audit; every kind was
// measured and fixed server-side, this fallback chain is defence in depth) or
// a counterparty city not currently FOW-visible to this player. Exported
// (pure, no DOM) for direct unit testing — same convention as ui/search.js's
// enterTargetIndex.
export function resolveDestination(kind, payload) {
  if (!payload) return null;
  if (payload.q != null && payload.r != null) return { q: payload.q, r: payload.r };
  const settlementID = COUNTERPARTY_KINDS.has(kind)
    ? (payload.counterparty_id || payload.settlement_id)
    : (payload.settlement_id || payload.dest_id || payload.destination_id || payload.threatens_settlement_id);
  if (settlementID) {
    const prov = (State.provinceData || []).find(p => p.id === settlementID || p.settlement_id === settlementID);
    if (prov) return { q: prov.q, r: prov.r };
  }
  const unitID = payload.unit_id || payload.ship_id;
  if (unitID) {
    const u = (State.unitsData || []).find(u => u.id === unitID)
      || (State.foreignUnitData || []).find(u => u.id === unitID);
    if (u && u.q != null && u.r != null) return { q: u.q, r: u.r };
  }
  return null;
}

export function closeDispatchWindow() {
  const el = document.getElementById('dispatch-window-overlay');
  if (el) el.classList.remove('open');
}

export function openDispatchWindow(kind, payload, timeLabel) {
  payload = payload || {};
  const overlay = document.getElementById('dispatch-window-overlay');
  const body = document.getElementById('dw-body');
  if (!overlay || !body) return;

  const dest = resolveDestination(kind, payload);
  const grainLine = kind === 'ColonyFounded' ? colonyFoundedGrainLine(payload) : '';

  body.innerHTML = `
    <div class="dw-row">
      <span class="dw-icon">${notifIcon(kind)}</span>
      <span class="dw-text">${notifText(kind, payload)}</span>
    </div>
    ${grainLine ? `<div class="dw-grain">${grainLine}</div>` : ''}
    ${timeLabel ? `<div class="dw-time">${timeLabel}</div>` : ''}
    <button class="dw-goto-btn" id="dw-goto-btn" ${dest ? '' : 'disabled title="No known location for this dispatch"'}>⌖ Take me there</button>
    <label class="dw-mute-row">
      <input type="checkbox" id="dw-mute-chk">
      Stop these as dispatches (still kept in Notifications)
    </label>
  `;
  overlay.classList.add('open');

  document.getElementById('dw-goto-btn').addEventListener('click', () => {
    if (dest) window.centreOn(dest.q, dest.r);
    closeDispatchWindow();
  });

  const chk = document.getElementById('dw-mute-chk');
  // Read the live preference every open (§6) — a dispatch having just fired
  // says nothing about whether the kind is muted right now, and an archive
  // row can be opened long after the mute state last changed.
  fetchAuth('/api/v1/notification-preferences')
    .then(r => r.ok ? r.json() : null)
    .then(d => { if (d) chk.checked = (d.muted_kinds || []).includes(kind); })
    .catch(() => {});
  chk.addEventListener('change', () => {
    const method = chk.checked ? 'PUT' : 'DELETE';
    fetchAuth(`/api/v1/notification-preferences/${encodeURIComponent(kind)}`, { method }).catch(() => {});
  });
}
