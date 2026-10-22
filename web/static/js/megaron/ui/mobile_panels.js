// Mobile window ownership. Each surface retains its own close/cleanup lifecycle.
const panels = new Map();
let active = null;
const mark = (el, name, on) => el?.classList?.[on ? 'add' : 'remove']?.(name);
const narrow = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 480px)').matches;

function sync() {
  const mobile = narrow();
  for (const [el] of panels) mark(el, 'mobile-fullscreen', !!mobile);
  const blocked = !!mobile && panels.size > 0;
  const map = document.getElementById('map-root');
  mark(map, 'mobile-panel-blocked', blocked);
  const canvas = document.getElementById('hex-canvas');
  if (canvas) canvas.inert = blocked;
  mark(document.body, 'mobile-panel-open', blocked);
}
export function openMobilePanel(el, close) {
  if (!el) return;
  if (narrow()) {
    for (const [other, cleanup] of [...panels]) {
      if (other !== el) { cleanup(); panels.delete(other); mark(other, 'mobile-fullscreen', false); }
    }
  }
  panels.set(el, close);
  active = el;
  sync();
}
export function closeMobilePanel(el) {
  if (!el) return;
  panels.delete(el);
  mark(el, 'mobile-fullscreen', false);
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
