import { State } from '../state.js';
import { fetchAuth } from '../api.js';

// ── Persistent notifications (bell badge) ──────────────────────────────────
export function updateNotifBadge(count) {
  const badge = document.getElementById('gt-notif-badge');
  if (count > 0) {
    badge.textContent = count > 99 ? '99+' : String(count);
    badge.style.display = 'inline';
  } else {
    badge.style.display = 'none';
  }
}

// Fetches the initial unread count on load. Needs State.WORLD_ID, so main.js
// calls this only after bootstrap() has populated State (see main.js init
// order) — the original was a self-running IIFE at the bottom of the single
// script, which worked only because WORLD_ID was already set by the FAS 1
// inline bootstrap loader before the classic script was even injected.
export async function initNotifications() {
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/notifications?unread=true`);
    if (r.ok) {
      const data = await r.json();
      updateNotifBadge(data.unread || 0);
    }
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

function dismissChip(chip) {
  if (chip.classList.contains('dismissing')) return;
  chip.classList.add('dismissing');
  chip.addEventListener('animationend', () => { chip.remove(); recomputeChips(); }, { once: true });
}

// Dismiss every dispatch currently on the strip. Timothy 2026-08-22: "'ta bort
// allt' är mer motiverat för den andra notislistan, där de ramlar in högerifrån."
// This is the transient stream, so clearing it destroys nothing — every dispatch
// is also a row in the notifications drawer, which is the permanent archive (see
// ui/drawers/notif.js). The strip is empty on reload regardless.
export function dismissAllChips() {
  const strip = document.getElementById('gt-dispatch-strip');
  if (!strip) return;
  strip.querySelectorAll('.dispatch-chip:not(.dismissing)').forEach(dismissChip);
}

const DOMAIN_DRAWER = {
  war: 'war', city: 'city', trade: 'diplomacy',
  diplomacy: 'diplomacy', kult: 'kult', system: null,
};

// openDrawer is the generic drawer-chrome dispatcher owned by main.js (the
// topmost layer) — this module cannot import it without an upward/cyclical
// dependency, so it goes through the window bridge main.js sets up, same
// convention as the canvas → drawer calls in render/map.js.
export function addDispatch(domain, glyph, text, time) {
  const strip = document.getElementById('gt-dispatch-strip');
  const chip  = document.createElement('div');
  chip.className  = 'dispatch-chip dc-' + domain;
  chip.title      = text;
  chip.innerHTML  = `
    <span class="dc-icon"><span class="dc-dot"></span><span class="dc-glyph">${glyph}</span></span>
    <span class="dc-text">${text}</span>
    <span class="dc-time">${time}</span>
    <button class="dc-x" title="Dismiss">✕</button>
  `;
  chip.addEventListener('click', function(e) {
    if (e.target.classList.contains('dc-x')) return;
    const target = DOMAIN_DRAWER[domain];
    if (target) window.openDrawer(target);
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
