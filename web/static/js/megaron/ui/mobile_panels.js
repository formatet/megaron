// Mobile window ownership. Each surface retains its own close/cleanup lifecycle.
const panels = new Map();
const sheets = new Set();
const popups = new Set();
let active = null;
const mark = (el, name, on) => el?.classList?.[on ? 'add' : 'remove']?.(name);
const narrow = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 480px)').matches;

function sync() {
  const mobile = narrow();
  for (const [el] of panels) {
    mark(el, 'mobile-fullscreen', !!mobile && !sheets.has(el));
    mark(el, 'mobile-sheet', !!mobile && sheets.has(el));
  }
  const picking = typeof document !== 'undefined' && document.body?.classList?.contains?.('hex-picking');
  const blocked = !!mobile && [...panels.keys()].some(el => !sheets.has(el)) && !picking;
  const map = document.getElementById('map-root');
  mark(map, 'mobile-panel-blocked', blocked);
  const canvas = document.getElementById('hex-canvas');
  if (canvas) canvas.inert = blocked;
  mark(document.body, 'mobile-panel-open', blocked);
}
const clear = el => { sheets.delete(el); popups.delete(el); mark(el, 'mobile-fullscreen', false); mark(el, 'mobile-sheet', false); };
// { sheet: true } = half-height bottom sheet on a phone: the map stays live above it.
// { popup: true } = a small menu at the pointer (the march menu): on desktop it covers what it sits on without closing it.
export function openMobilePanel(el, close, { sheet = false, popup = false } = {}) {
  if (!el) return;
  if (narrow()) {
    for (const [other, cleanup] of [...panels]) {
      if (other !== el) { cleanup(); panels.delete(other); clear(other); }
    }
  }
  if (sheet) sheets.add(el); else sheets.delete(el);
  if (popup) popups.add(el); else popups.delete(el);
  panels.set(el, close);
  active = el;
  sync();
  // Desktop: the caller shows `el` right after this returns, so judge the overlap once it has its place.
  if (!narrow()) queueMicrotask(() => closeCovered(el));
}
// ── Desktop: a window that opens on top of another closes it ───────────────────────────────────────────
// Two windows that do not overlap both stay. The covered one goes through its own close callback, so its
// state is really cleared (a hidden-not-closed panel comes back later). The newest window wins, also when
// it sits beneath the old one (z-index) — a stale window must not cover what the player just opened.
export function rectsOverlap(a, b) {
  if (!a || !b || !(a.right > a.left) || !(a.bottom > a.top) || !(b.right > b.left) || !(b.bottom > b.top)) return false;
  return a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;
}
// The visible box: the dialog inside a modal overlay (not its backdrop), a sliding drawer at its resting place.
function boxOf(el) {
  const t = el?.querySelector?.(':scope > .search-box, :scope > .dispatch-window') || el;
  const r = t?.getBoundingClientRect?.();
  if (!r) return null;
  let dx = 0;
  try { if (typeof getComputedStyle === 'function') dx = new DOMMatrix(getComputedStyle(t).transform).m41 || 0; } catch (_) { /* no transform */ }
  return { left: r.left - dx, right: r.right - dx, top: r.top, bottom: r.bottom };
}
function closeCovered(el) {
  if (!panels.has(el) || popups.has(el) || narrow()) return;
  const mine = boxOf(el);
  for (const [other, close] of [...panels]) {
    if (other === el || !rectsOverlap(mine, boxOf(other))) continue;
    close(); panels.delete(other); clear(other);
  }
  sync();
}
export function closeMobilePanel(el) {
  if (!el) return;
  panels.delete(el);
  clear(el);
  if (active === el) active = [...panels.keys()].at(-1) || null;
  sync();
}
export function refreshMobilePanels() {
  if (narrow() && active) {
    for (const [el, close] of [...panels]) if (el !== active) { close(); panels.delete(el); }
  }
  sync();
}
if (typeof window !== 'undefined') window.addEventListener?.('resize', refreshMobilePanels);

// Upward pan (negative, canvas px) that lifts a hex at canvas y `hexY` clear of a bottom sheet
// `sheetH` tall in a map `mapH` tall; `pad` is the hex's half-height plus a margin. 0 = already visible.
export function sheetPanDelta(hexY, mapH, sheetH, pad) {
  if (!(sheetH > 0) || hexY < 0) return 0;
  const over = hexY + pad - (mapH - sheetH);
  return over > 0 ? -over : 0;
}
