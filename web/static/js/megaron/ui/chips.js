import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { openDispatchWindow } from './dispatch_window.js';
import { notifText, notifIcon, notifDomain, fmtAgo, colonyFoundedGrainLine } from './format.js';

// ── Persistent notifications (bell badge) ──────────────────────────────────
// unreadCount mirrors the badge so dismissing a chip can decrement it without
// a round trip per dismissal. ws.js refetches the true count on every archived
// kind, so a drift here self-corrects on the next event.
let unreadCount = 0;

export function updateNotifBadge(count) {
  unreadCount = Math.max(0, count);
  const badge = document.getElementById('gt-notif-badge');
  if (unreadCount > 0) {
    badge.textContent = unreadCount > 99 ? '99+' : String(unreadCount);
    badge.style.display = 'inline';
  } else {
    badge.style.display = 'none';
  }
}

// How many unread notifications are rebuilt as chips on load. The strip is one
// row of a top bar: below ~12 the chips still carry readable text at the widths
// recomputeChips hands out, above it they collapse to icon-only stubs and the
// field stops being a work surface. The rest are not lost — they are in the
// Notifications archive, which is where a backlog belongs.
const HYDRATE_MAX = 12;

// Fetches the unread count AND rebuilds the Dispatches strip from the archive.
// Needs State.WORLD_ID, so main.js calls this only after bootstrap() has
// populated State (see main.js init order).
//
// Rebuilding the strip on load is the whole reason a returning Wanax has
// anything to work through: chips used to be born only from a live WebSocket
// push, so nine hours away meant logging in to an EMPTY field — the one surface
// meant to carry the return (megaron_arbetssatt.md §14 grind 2). The archive is
// the source: ?dispatchable=true asks the server for exactly what a live push
// would have delivered, so a muted kind stays muted across a reload.
export async function initNotifications() {
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/notifications?unread=true&dispatchable=true`);
    if (!r.ok) return;
    const data = await r.json();
    updateNotifBadge(data.unread || 0);
    // The server answers newest-first; the strip reads oldest-left so a Wanax
    // works through them in the order the runners actually arrived. Take the
    // newest HYDRATE_MAX, then reverse — dropping the OLDEST of a long backlog
    // rather than the newest, because the newest is the one still worth
    // answering (an incoming march has travel time left; last night's does not).
    (data.notifications || []).slice(0, HYDRATE_MAX).reverse().forEach(n => {
      const body = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
      addDispatch({ kind: n.kind, payload: body, id: n.id, level: n.level, time: fmtAgo(n.created_at) });
    });
  } catch (_) {}
}

// ── Dispatch chips ──────────────────────────────────────────────────────────
const MIN_W = 26;
const GAP   = 3;

// Below this many chips, dismissing one by one is not a chore and the extra
// control is just noise on the top bar. Tunable, not a rule.
const DISMISS_ALL_FROM = 3;

function recomputeChips() {
  const strip = document.getElementById('gt-dispatch-strip');
  const chips = [...strip.querySelectorAll('.dispatch-chip:not(.dismissing)')];
  // Toggled BEFORE the empty-strip early return below — otherwise the button
  // would survive the dismissal of the last chip it belongs to.
  const allBtn = document.getElementById('dc-dismiss-all');
  if (allBtn) allBtn.style.display = chips.length >= DISMISS_ALL_FROM ? '' : 'none';
  if (!chips.length) return;
  const n         = chips.length;
  const available = strip.clientWidth - 12 - (n - 1) * GAP;
  const totalW    = (n * (n + 1)) / 2;
  const extra     = Math.max(0, available - n * MIN_W);
  chips.forEach((chip, i) => {
    const weight = i + 1;
    const width  = Math.round(MIN_W + (weight / totalW) * extra);
    const depth  = n - 1 - i;
    const op     = Math.max(0.42, 1 - depth * 0.1);
    chip.style.maxWidth = width + 'px';
    chip.style.opacity  = op;
    const textEl = chip.querySelector('.dc-text');
    const timeEl = chip.querySelector('.dc-time');
    const xEl    = chip.querySelector('.dc-x');
    if (textEl) textEl.style.display = width <= 70  ? 'none' : '';
    if (timeEl) timeEl.style.display = width <= 115 ? 'none' : '';
    if (xEl)    xEl.style.display    = width <= 115 ? 'none' : '';
  });
}

// markRead tells the server this one notification has been dealt with, so the
// chip does not come back the next time initNotifications rebuilds the strip.
// Fire-and-forget: a dismissal is a UI gesture and must never block or fail
// visibly on bookkeeping. Chips without an id (a push that raced a failed
// archive insert) simply stay client-only, exactly as every chip used to be.
function markRead(id) {
  if (!id) return;
  updateNotifBadge(unreadCount - 1);
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/notifications/${id}/read`, { method: 'POST' }).catch(() => {});
}

function dismissChip(chip) {
  if (chip.classList.contains('dismissing')) return;
  chip.classList.add('dismissing');
  markRead(chip.dataset.notifId);
  chip.addEventListener('animationend', () => { chip.remove(); recomputeChips(); }, { once: true });
}

// Dismiss every dispatch currently on the strip. Timothy 2026-08-22: "'ta bort
// allt' är mer motiverat för den andra notislistan, där de ramlar in högerifrån."
// This is the transient stream, so clearing it destroys nothing — every dispatch
// is also a row in the notifications drawer, which is the permanent archive (see
// ui/drawers/notif.js). Dismissing marks each one read; the row stays.
export function dismissAllChips() {
  const strip = document.getElementById('gt-dispatch-strip');
  if (!strip) return;
  strip.querySelectorAll('.dispatch-chip:not(.dismissing)').forEach(dismissChip);
}

// addDispatch renders one dispatch chip from a notification — the SAME object
// the Notifications archive holds: {kind, payload, id, level, time}. Live
// pushes (ws.js) and the load-time rebuild (initNotifications) both go through
// here, so the two can never drift apart.
//
// Everything shown is derived from `kind` via ui/format.js (text, icon, colour
// family) rather than spelled out per branch by the caller. That is the fix for
// the old shape: ws.js named text/glyph/domain by hand in 19 branches, five of
// which were dead and 33 server kinds of which had no branch at all — so an
// event nobody remembered to wire was pushed and silently dropped. Now every
// kind renders; an unknown one gets a neutral chip and its generic text.
//
// A click opens the SAME window a Notifications archive row opens
// (megaron_plan_dispatches.md §1: "ett fönster, två dörrar").
export function addDispatch(n) {
  const strip = document.getElementById('gt-dispatch-strip');
  if (!strip) return;
  const kind    = n.kind;
  const payload = n.payload || {};
  const time    = n.time || 'now';
  // ColonyFounded's founding grain balance is the one line that must ride ON
  // the chip: a colony that cannot feed itself is a decision waiting, and the
  // one-line text alone would not say so.
  const grain = kind === 'ColonyFounded' ? colonyFoundedGrainLine(payload) : '';
  const text  = notifText(kind, payload) + (grain ? ' — ' + grain : '');

  const chip = document.createElement('div');
  chip.className = 'dispatch-chip dc-' + notifDomain(kind);
  chip.title     = text;
  if (n.id) chip.dataset.notifId = n.id;
  chip.innerHTML = `
    <span class="dc-icon"><span class="dc-dot"></span><span class="dc-glyph">${notifIcon(kind)}</span></span>
    <span class="dc-text">${text}</span>
    <span class="dc-time">${time}</span>
    <button class="dc-x" title="Dismiss">✕</button>
  `;
  chip.addEventListener('click', function(e) {
    if (e.target.classList.contains('dc-x')) return;
    openDispatchWindow(kind, payload, time);
    dismissChip(this);
  });
  chip.addEventListener('contextmenu', function(e) {
    e.preventDefault();
    dismissChip(this);
  });
  chip.querySelector('.dc-x').addEventListener('click', function(e) {
    e.stopPropagation();
    dismissChip(chip);
  });
  strip.appendChild(chip);
  recomputeChips();
}

window.addEventListener('resize', recomputeChips);
