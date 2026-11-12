// Mobile window ownership. Each surface retains its own close/cleanup lifecycle.
const panels = new Map();
const sheets = new Set();
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
const clear = el => { sheets.delete(el); mark(el, 'mobile-fullscreen', false); mark(el, 'mobile-sheet', false); };
// { sheet: true } = half-height bottom sheet on a phone: the map stays live above it.
export function openMobilePanel(el, close, { sheet = false } = {}) {
  if (!el) return;
  if (narrow()) {
    for (const [other, cleanup] of [...panels]) {
      if (other !== el) { cleanup(); panels.delete(other); clear(other); }
    }
  }
  if (sheet) sheets.add(el); else sheets.delete(el);
  panels.set(el, close);
  active = el;
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
