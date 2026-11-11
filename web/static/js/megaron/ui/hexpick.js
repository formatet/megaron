// One shared "pick a hex on the map" mode (Redirect, Wake the Storm). While it runs, every panel that could
// cover the map is hidden — not closed, so typed text, scroll and tabs survive — and a one-line bar at the top of
// the map holds the prompt and Cancel. First tap on a hex selects it, the bar then offers "Use this hex"; another
// tap moves the selection; the button confirms. The map hands its plain taps to pickTap() (render/map.js).
import { State } from '../state.js';
import { refreshMobilePanels } from './mobile_panels.js';
import { esc } from './format.js';

// The state machine, free of the DOM. env = { hide() -> token, restore(token), render(view|null), describe(q,r) -> {name?} }.
// view = { prompt, sel: {q,r,name}|null, reason: string|null, canConfirm, shortcuts: [{label,q,r}] }.
// shortcuts (optional, may arrive after the pick starts via setShortcuts) are named hexes the bar offers behind "Cities".
export function createPicker(env) {
  let cur = null;
  const view = () => cur && { prompt: cur.prompt, sel: cur.sel, reason: cur.reason, canConfirm: !!cur.sel && !cur.reason, shortcuts: cur.shortcuts };
  function finish(value) {
    const c = cur; cur = null;
    env.restore(c.token);
    env.render(null);
    c.resolve(value);
  }
  return {
    active: () => !!cur,
    view,
    pick({ prompt, accept, shortcuts } = {}) {
      if (cur) finish(null); // only one pick at a time
      return new Promise(resolve => {
        cur = { prompt: prompt || 'Tap a hex', accept: accept || (() => null), shortcuts: shortcuts || [], sel: null, reason: null, resolve, token: env.hide() };
        env.render(view());
      });
    },
    tap(q, r) {
      if (!cur) return;
      const d = env.describe(q, r) || {};
      const reason = cur.accept({ q, r }) || null;
      cur.sel = { q, r, name: d.name || '' };
      cur.reason = typeof reason === 'string' && reason ? reason : null;
      env.render(view());
    },
    setShortcuts(list) { if (cur) { cur.shortcuts = list || []; env.render(view()); } },
    shortcut(i) {
      const s = cur && cur.shortcuts[i];
      if (!s) return;
      this.tap(s.q, s.r);
      this.confirm();
    },
    confirm() {
      if (!cur || !cur.sel || cur.reason) return;
      finish({ q: cur.sel.q, r: cur.sel.r });
    },
    cancel() { if (cur) finish(null); },
  };
}

// ── The page: bar element, body class, map marker ────────────────────────────
function barHTML(v) {
  const s = v.sel;
  const where = s ? '(' + s.q + ',' + s.r + ')' + (s.name ? ' ' + esc(s.name) : '') : '';
  const mid = !s ? ''
    : v.reason ? '<span class="hexpick-reason">' + where + ' — ' + esc(v.reason) + '</span>'
    : '<button type="button" class="hexpick-ok" data-pick-ok>' + where + ' — Use this hex</button>';
  const cities = v.shortcuts && v.shortcuts.length ? '<button type="button" class="hexpick-cities" data-pick-cities>Cities</button>' : '';
  return '<span class="hexpick-prompt">' + esc(v.prompt) + '</span>'
    + '<span class="hexpick-actions">' + mid + cities + '<button type="button" class="hexpick-cancel" data-pick-cancel>Cancel</button></span>';
}

// The Cities sheet under the bar: one button per shortcut, scrolls inside 40 % of the screen.
function sheetHTML(v) {
  return (v.shortcuts || []).map((s, i) => '<button type="button" class="hexpick-city" data-pick-shortcut="' + i + '">' + esc(s.label) + '</button>').join('');
}

const domEnv = {
  hide() {
    document.body.classList.add('hex-picking');
    const canvas = document.getElementById('hex-canvas');
    if (canvas) canvas.inert = false; // a phone panel had made the map inert
    return null;
  },
  restore() {
    document.body.classList.remove('hex-picking');
    State.pickHexSel = null; State.dirty = true;
    refreshMobilePanels();
  },
  render(v) {
    let bar = document.getElementById('hexpick-bar');
    if (!v) { if (bar) bar.remove(); const sh = document.getElementById('hexpick-sheet'); if (sh) sh.remove(); State.pickHexSel = null; State.dirty = true; return; }
    if (!bar) {
      bar = document.createElement('div');
      bar.id = 'hexpick-bar'; bar.className = 'hexpick-bar'; bar.setAttribute('role', 'status');
      bar.addEventListener('click', e => {
        if (e.target.closest('[data-pick-ok]')) picker.confirm();
        else if (e.target.closest('[data-pick-cities]')) { const sh = document.getElementById('hexpick-sheet'); if (sh) sh.hidden = !sh.hidden; }
        else if (e.target.closest('[data-pick-cancel]')) picker.cancel();
      });
      (document.getElementById('map-root') || document.body).appendChild(bar);
    }
    bar.innerHTML = barHTML(v);
    let sheet = document.getElementById('hexpick-sheet');
    if (!v.shortcuts || !v.shortcuts.length) { if (sheet) sheet.remove(); }
    else {
      if (!sheet) {
        sheet = document.createElement('div');
        sheet.id = 'hexpick-sheet'; sheet.className = 'hexpick-sheet'; sheet.hidden = true;
        sheet.addEventListener('click', e => {
          const b = e.target.closest('[data-pick-shortcut]');
          if (b) picker.shortcut(Number(b.dataset.pickShortcut));
        });
        bar.after(sheet);
      }
      sheet.innerHTML = sheetHTML(v);
      sheet.style.top = bar.offsetHeight + 'px';
    }
    State.pickHexSel = v.sel ? { q: v.sel.q, r: v.sel.r } : null;
    State.dirty = true;
  },
  describe(q, r) {
    const p = (State.provinceData || []).find(p => p.q === q && p.r === r);
    return { name: p && p.name || '' };
  },
};
const picker = createPicker(domEnv);

export const pickHex = opts => picker.pick(opts);
export const pickTap = (q, r) => picker.tap(q, r);
export const pickActive = () => picker.active();
export const pickCancel = () => picker.cancel();
export const pickShortcuts = list => picker.setShortcuts(list);
export { barHTML, sheetHTML };
