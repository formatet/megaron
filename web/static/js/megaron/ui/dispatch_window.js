import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { notifText, notifIcon, colonyFoundedGrainLine, formatApiError } from './format.js';
import { codexArticleForKind, openCodex } from './codex.js';
import { fmtArrival } from './time.js';

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

// ── Occupation choices (S3/S6, megaron_plan_erovring.md) ───────────────────
// keryx (`keryx occupation order`) has carried sack/burn/annex since S3; the
// web had zero call sites until this slice (Timothy 2026-09-25). The Dispatch
// window is where a Wanax already meets an occupied city they hold: CityOccupied
// (role:"attacker") fires the instant the battle ends and carries the server's
// own `choices` list — ["occupy","sack","burn"], occupation.go's occupySettlement
// — annex isn't offered yet because the hold hasn't matured. CityAnnexReady
// fires once occupation_check.go's counter matures and adds annex; sack/burn
// stay available too (nothing revokes them once annex unlocks). "occupy" means
// leave it as it is — the default — so it gets no button, closing the window
// already does that.
export function occupationChoicesFor(kind, payload) {
  if (kind === 'CityOccupied' && payload && payload.role === 'attacker') {
    return (payload.choices || []).filter(a => a === 'sack' || a === 'burn');
  }
  if (kind === 'CityAnnexReady') {
    return ['sack', 'burn', 'annex'];
  }
  return [];
}

// One plain consequence line per choice (the brief: "each choice states its
// consequence"). burn/annex are irreversible and get an inline confirm step
// (never window.confirm — it blocks browser automation).
const OCCUPATION_INFO = {
  sack:  { label: 'Sack',  desc: 'Loot the city — silver and a share of its goods. It stays theirs.', confirm: false },
  burn:  { label: 'Burn',  desc: 'Sack it, then raze it. Nobody may resettle it for a long while.', confirm: true },
  annex: { label: 'Annex', desc: 'Keep it for good.', confirm: true },
};

// buildOccupationOrderBody — the POST .../occupation-order request body
// (api/handlers/unit.go OccupationOrder). Sack always loots every lootable
// good, same as keryx's own --goods default: a picker would be one more
// decision on top of an already weighty choice, so this offers it simply.
export function buildOccupationOrderBody(action) {
  return { action };
}

function occupationBlockHTML(choices) {
  const rows = choices.map(a => {
    const info = OCCUPATION_INFO[a];
    return `
      <div class="dw-occ-choice">
        <button class="dw-occ-btn" data-occ-action="${a}">${info.label}</button>
        <span class="dw-occ-desc">${info.desc}</span>
      </div>
      ${info.confirm ? `
      <div class="dw-occ-confirm" id="dw-occ-confirm-${a}" hidden>
        <span class="dw-occ-warn">This cannot be undone.</span>
        <button class="dw-occ-btn" data-occ-confirm="${a}">Yes, ${info.label.toLowerCase()} it</button>
        <button class="dw-occ-btn" data-occ-cancel="${a}">Cancel</button>
      </div>` : ''}`;
  }).join('');
  return `<div class="dw-occupation" id="dw-occupation">
    <div class="dw-occ-label">Decide the city's fate:</div>
    ${rows}
    <div class="dw-occ-result" id="dw-occ-result"></div>
  </div>`;
}

async function sendOccupationOrder(settlementID, action, resultEl, allBtns) {
  allBtns.forEach(b => { b.disabled = true; });
  if (resultEl) resultEl.textContent = 'Sending…';
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/${settlementID}/occupation-order`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildOccupationOrderBody(action)),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok) {
      if (resultEl) resultEl.textContent = formatApiError(data, 'Order failed');
      allBtns.forEach(b => { b.disabled = false; });
      return;
    }
    if (resultEl) resultEl.textContent = `Runner sent — reaches the city ${fmtArrival(data.courier_arrives_at)}.`;
  } catch (_) {
    if (resultEl) resultEl.textContent = 'Order failed — network error.';
    allBtns.forEach(b => { b.disabled = false; });
  }
}

function wireOccupationBlock(choices, settlementID) {
  const root = document.getElementById('dw-occupation');
  if (!root) return;
  const resultEl = document.getElementById('dw-occ-result');
  const allBtns = () => Array.from(root.querySelectorAll('button'));

  choices.forEach(a => {
    const info = OCCUPATION_INFO[a];
    const btn = root.querySelector(`[data-occ-action="${a}"]`);
    if (!btn) return;
    if (!info.confirm) {
      btn.addEventListener('click', () => sendOccupationOrder(settlementID, a, resultEl, allBtns()));
      return;
    }
    const confirmRow = document.getElementById(`dw-occ-confirm-${a}`);
    btn.addEventListener('click', () => { confirmRow.hidden = false; btn.disabled = true; });
    confirmRow.querySelector(`[data-occ-confirm="${a}"]`).addEventListener('click', () =>
      sendOccupationOrder(settlementID, a, resultEl, allBtns()));
    confirmRow.querySelector(`[data-occ-cancel="${a}"]`).addEventListener('click', () => {
      confirmRow.hidden = true;
      btn.disabled = false;
    });
  });
}

export function openDispatchWindow(kind, payload, timeLabel) {
  payload = payload || {};
  const overlay = document.getElementById('dispatch-window-overlay');
  const body = document.getElementById('dw-body');
  if (!overlay || !body) return;

  const dest = resolveDestination(kind, payload);
  const grainLine = kind === 'ColonyFounded' ? colonyFoundedGrainLine(payload) : '';
  // Third door into the Codex (megaron_plan_kodex.md): every dispatch kind
  // the index maps gets a link to the article that explains it.
  const article = codexArticleForKind(kind);
  const occChoices = occupationChoicesFor(kind, payload);
  const settlementID = payload.settlement_id;
  const showOcc = occChoices.length > 0 && !!settlementID;

  body.innerHTML = `
    <div class="dw-row">
      <span class="dw-icon">${notifIcon(kind)}</span>
      <span class="dw-text">${notifText(kind, payload)}</span>
    </div>
    ${grainLine ? `<div class="dw-grain">${grainLine}</div>` : ''}
    ${timeLabel ? `<div class="dw-time">${timeLabel}</div>` : ''}
    ${showOcc ? occupationBlockHTML(occChoices) : ''}
    <button class="dw-goto-btn" id="dw-goto-btn" ${dest ? '' : 'disabled title="No known location for this dispatch"'}>⌖ Take me there</button>
    ${article ? '<button class="dw-goto-btn dw-codex-btn" id="dw-codex-btn">? Read about this</button>' : ''}
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

  if (article) {
    document.getElementById('dw-codex-btn').addEventListener('click', () => {
      closeDispatchWindow();
      openCodex(article);
    });
  }

  if (showOcc) wireOccupationBlock(occChoices, settlementID);

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
