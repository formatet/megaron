// main.js — the map client's only entry point (loaded by web/static/map.html
// as <script type="module">). Responsibilities, in order:
//   1. bootstrap(): fetch what the old Go-template used to inline
//      (WORLD_ID, MY_PLAYER_ID, MY_SETTLEMENT_ID, WORLD_CREATED_AT,
//      TIME_SCALE, world name, diplomacy badge) into State.
//   2. Window exposures: every identifier referenced from an inline
//      on*="..." handler (in the static shell or in module template
//      literals), plus the window-bridge functions lower layers call to
//      reach higher ones without upward imports.
//   3. init calls, in dependency order (everything that used to run at
//      script load in the old single <script> and needs State populated).
import { BASE } from './config.js';
import { State } from './state.js';
import { serverNow } from './clock.js';
import { initWS, fullResync, checkWsLiveness } from './ws.js';
import {
  initMap, refreshTiles, loadMap, zoom, resetView, toggleActivityOverlay,
  closeInspect, sendMessengerFromInspect,
} from './render/map.js';
import { stopCityAnim } from './render/city.js';
import {
  showLawagatasBrief, dismissBrief, MusicPlayer, toggleMusic,
  initCelestial,
} from './ui/misc.js';
import { updateNotifBadge, initNotifications, addNotifChip } from './ui/chips.js';
import { toggleSearch, closeSearch, centreOn } from './ui/search.js';
import {
  closeMarchCtx, onColonizeToggle, openMarchCtx, sendMarch,
  renderColonizePreviewHTML,
} from './ui/marchctx.js';
import {
  loadCityDrawer, cycleCityView, saveLaborAlloc, startBuild, startCraft,
  loadTicklog, cancelBuild,
} from './ui/drawers/city.js';
import {
  loadWarDrawer, warRecruitFromUI, warRecruitShip, warDisband, warAbandon,
  unitRecall, unitRedirect, unitRedirectToggle, unitMarch, unitMarchSend,
  closeMarchPanel, unitStance, unitLoadPrompt, unitUnload, warFocusUnit,
} from './ui/drawers/war.js';
import {
  loadEconomyDrawer, loadTransferGoods, startTransfer,
} from './ui/drawers/economy.js';
import { loadKultDrawer, okRite } from './ui/drawers/kult.js';
import {
  loadDiplomacyDrawer, dipToggleKind, dipToggleThread, dipSendInThread,
  dipCancel, dipAccept, dipDecline, dipReply, dipComposeToggleKind, dipSend,
} from './ui/drawers/diplomacy.js';
import { loadNotifDrawer, notifShowKind } from './ui/drawers/notif.js';

// ── Drawer system (generic chrome — per-drawer content lives in ui/drawers/) ─
export function toggleDrawer(name) {
  if (State.activeDrawer === name) { closeDrawer(name); return; }
  if (State.activeDrawer) closeDrawer(State.activeDrawer);
  openDrawer(name);
}

export function openDrawer(name) {
  const el = document.getElementById('drawer-' + name);
  const tr = document.getElementById('trig-' + name);
  if (!el) return;
  el.classList.add('open');
  if (tr) tr.classList.add('active');
  document.getElementById('map-dim').classList.add('visible');
  State.activeDrawer = name;
  showLawagatasBrief(name);
  loadDrawerContent(name);
}

export function closeDrawer(name) {
  const el = document.getElementById('drawer-' + name);
  const tr = document.getElementById('trig-' + name);
  if (el) el.classList.remove('open');
  if (tr) tr.classList.remove('active');
  document.getElementById('map-dim').classList.remove('visible');
  State.activeDrawer = null;
  if (name === 'city') stopCityAnim();
}

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    if (State.activeDrawer) { closeDrawer(State.activeDrawer); return; }
    document.getElementById('search-overlay').classList.remove('open');
  }
});

// Drawer content dispatch — was one big loadDrawerContent(name) with the city
// branch inline; each branch now lives in its drawer module.
async function loadDrawerContent(name) {
  if (name === 'city') {
    await loadCityDrawer();
  } else if (name === 'war') {
    await loadWarDrawer();
  } else if (name === 'diplomacy') {
    await loadDiplomacyDrawer();
  } else if (name === 'economy') {
    await loadEconomyDrawer();
  } else if (name === 'kult') {
    await loadKultDrawer();
  } else if (name === 'notif') {
    await loadNotifDrawer();
  }
}

// reloadActiveDrawer — window-bridge target for ws.js: rebuild the open drawer so
// it reflects fresh data after a WS event / resync. Drawer renderers already
// rebuild innerHTML idempotently. Guard: skip if the user is mid-entry in a field
// inside the drawer, so a live update never clobbers a recruit/march form.
function reloadActiveDrawer() {
  if (!State.activeDrawer) return;
  const drawerEl = document.getElementById('drawer-' + State.activeDrawer);
  const ae = document.activeElement;
  if (drawerEl && ae && drawerEl.contains(ae) && /^(INPUT|SELECT|TEXTAREA)$/.test(ae.tagName)) return;
  loadDrawerContent(State.activeDrawer);
}

// A tab returning to the foreground may have missed WS events while hidden (and
// its socket may be silently dead). Drop a dead socket at once, then refetch
// everything a page load would — the reconnect's onopen resync covers the rest.
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') {
    checkWsLiveness();
    fullResync();
  }
});

// ── Window exposures ──────────────────────────────────────────────────────
// (a) Inline-handler targets: every name referenced from an on*="..."
//     attribute in the static shell or in a module's template-literal HTML.
//     Sorted alphabetically. Verified by the check script in the FAS 2 gates.
// (b) Window-bridge: functions lower layers call upward via window.* instead
//     of an upward import (ws.js → chips/misc/map; render/map.js → marchctx/
//     drawer chrome; chips.js → openDrawer).
Object.assign(window, {
  // (a) inline-handler targets
  cancelBuild,
  centreOn,
  closeDrawer,
  closeInspect,
  closeMarchCtx,
  closeMarchPanel,
  closeSearch,
  cycleCityView,
  dipAccept,
  dipCancel,
  dipComposeToggleKind,
  dipDecline,
  dipReply,
  dipSend,
  dipSendInThread,
  dipToggleKind,
  dipToggleThread,
  dismissBrief,
  loadTicklog,
  loadTransferGoods,
  loadWarDrawer,
  notifShowKind,
  okRite,
  onColonizeToggle,
  resetView,
  saveLaborAlloc,
  sendMarch,
  sendMessengerFromInspect,
  startBuild,
  startCraft,
  startTransfer,
  toggleActivityOverlay,
  toggleDrawer,
  toggleMusic,
  toggleSearch,
  unitLoadPrompt,
  unitMarch,
  unitMarchSend,
  unitRecall,
  unitRedirect,
  unitRedirectToggle,
  unitStance,
  unitUnload,
  warAbandon,
  warDisband,
  warRecruitFromUI,
  warRecruitShip,
  zoom,
  // (b) window-bridge (not inline-handler targets)
  MusicPlayer,
  addNotifChip,
  openDrawer,
  openMarchCtx,
  refreshTiles,
  reloadActiveDrawer,
  renderColonizePreviewHTML,
  updateNotifBadge,
  warFocusUnit,
});

// ── Bootstrap — fetch what the Go-template used to inline ─────────────────
// See the FAS 1 plan for the server-fact derivation behind each value.
async function bootstrap() {
  const token = localStorage.getItem('poleia_token');
  const worldID = location.pathname.split('/')[2]; // URL is always /world/{id}/map

  async function get(path) {
    const res = await fetch(BASE + path, {
      headers: token ? { Authorization: 'Bearer ' + token } : {},
    });
    if (!res.ok) throw new Error(path + ' → ' + res.status);
    return res.json();
  }

  State.WORLD_ID = worldID;
  State.TIME_SCALE = 1; // MapView hårdkodar 1 (web.go) — följ servern om det ändras

  let me;
  try {
    me = await get('/api/v1/auth/me');
  } catch (e) {
    location.href = '/';
    return false;
  }
  State.MY_PLAYER_ID = me.id;

  try {
    const [world, provinces] = await Promise.all([
      get('/api/v1/worlds/' + worldID),
      get('/api/v1/worlds/' + worldID + '/provinces'),
    ]);
    State.WORLD_CREATED_AT = world.created_at;
    document.title = 'MEGARON — ' + world.name;

    // Tick anchor for local ETA math (ui/time.js, K4 contract). serverNow()
    // may still be un-anchored this early — bootstrap uses bare fetch(), so
    // no Date header has passed through noteServerDate yet — but the skew is
    // bounded by clock.js's noise floor and self-corrects at first re-anchor.
    if (world.current_tick != null && world.tick_seconds > 0) {
      State.CURRENT_TICK   = world.current_tick;
      State.TICK_SECONDS   = world.tick_seconds;
      State.TICK_ANCHOR_MS = serverNow();
      // Dev tempo label: at production cadence 1 tick = 1 game hour takes a
      // real hour; anything under a real minute per tick is a test world.
      if (world.tick_seconds < 60) {
        const el = document.getElementById('gt-devtempo');
        if (el) {
          el.textContent = 'Test world — time runs ' + Math.round(3600 / world.tick_seconds) + '× faster';
          el.style.display = '';
        }
      }
    }

    const capital = provinces.find(p => p.own && p.is_capital);
    State.MY_SETTLEMENT_ID = capital ? capital.settlement_id : '';
  } catch (e) {
    console.error('bootstrap: world/provinces fetch failed', e);
    State.WORLD_CREATED_AT = '';
    State.MY_SETTLEMENT_ID = '';
  }

  // Founder phase (Nomadic Host): active → the map shows the Host panel and
  // founding affordances instead of city surfaces. Cleared on settle.
  try {
    const fp = await get('/api/v1/worlds/' + worldID + '/founding/status');
    State.founderPhase = fp.active ? fp : null;
  } catch (e) {
    console.error('bootstrap: founding status fetch failed', e);
    State.founderPhase = null;
  }

  // Diplomacy badge — non-blocking.
  get('/api/v1/worlds/' + worldID + '/messengers/inbox')
    .then(inbox => {
      const badge = document.getElementById('diplo-badge');
      if (badge && inbox.length) {
        badge.textContent = inbox.length;
        badge.style.display = '';
      }
    })
    .catch(e => console.error('bootstrap: inbox fetch failed', e));

  return true;
}

// ── Start ─────────────────────────────────────────────────────────────────
// Init order matters: State must be populated (bootstrap) before anything
// that reads State.WORLD_ID fires. Each init corresponds to top-level code
// that ran unconditionally in the old single <script>.
(async function start() {
  if (!(await bootstrap())) return; // 401 → redirected to /

  initMap();           // canvas input handlers + loadMap() + render loop + 30s/3s polls
  initWS();            // websocket connect + reconnect loop
  initCelestial();     // celestial clock + its 3 s interval
  initNotifications(); // initial unread-badge fetch
})();
