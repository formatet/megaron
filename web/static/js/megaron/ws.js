import { BASE } from './config.js';
import { State } from './state.js';
import { serverNow } from './clock.js';
import { fetchAuth } from './api.js';
import { track } from './telemetry.js';
import { notifText, notifIcon, colonyFoundedGrainLine } from './ui/format.js';

// ── WebSocket — real-time province updates ────────────────────────────────
// notifText/notifIcon/colonyFoundedGrainLine are pure formatting helpers
// (ui/format.js, no DOM/state deps of their own) so this module imports them
// directly. MusicPlayer, addNotifChip, refreshTiles and updateNotifBadge live
// in higher layers (ui/misc.js, ui/chips.js, render/map.js) that this module
// is not allowed to import per the config/state ← api/ws ← render ← ui ← main
// dependency order — those are reached via the window.* bridge that main.js
// sets up (same convention used for canvas → drawer calls in render/map.js).
// ── Module-scoped connection state ────────────────────────────────────────
// ws lives at module scope so the watchdog AND main.js's visibilitychange
// handler (via checkWsLiveness) can reach the live socket.
let ws = null;
let firstConnect = true;
// Wall-clock of the last close, so onopen can report the reconnect downtime as a
// WS-fix kvittensmätare (megaron_plan_umami.md). null before the first close.
let closedAt = null;
// Watchdog thresholds. The server pings + heartbeats every ~25 s (notify/hub.go);
// if nothing at all arrives for STALE_MS the path is dead (silent NAT/WG drop
// with no FIN), so force-close to trigger the reconnect loop.
const STALE_MS = 65000;
const WATCHDOG_MS = 20000;

// checkWsLiveness closes a socket gone silent past STALE_MS so onclose fires and
// reconnects. Runs on a timer AND on tab-visible (main.js) so a dead WS is
// dropped immediately on return instead of after up to one watchdog tick.
export function checkWsLiveness() {
  if (ws && ws.readyState === WebSocket.OPEN &&
      State.lastWsMsgAt && Date.now() - State.lastWsMsgAt > STALE_MS) {
    track('ws_dead_watchdog');
    ws.close();
  }
}

// Debounced bridge to main.js's reloadActiveDrawer — coalesces the burst of
// data-updating WS events into at most one drawer rebuild per second.
let drawerTimer = null;
function reloadDrawerDebounced() {
  clearTimeout(drawerTimer);
  drawerTimer = setTimeout(() => window.reloadActiveDrawer && window.reloadActiveDrawer(), 1000);
}

// Coalesce event-triggered refetches per endpoint: a burst of WS events collapses
// to one fetch per endpoint per ~2 s, so load scales with events, not events×
// clients. The 30 s poll (render/map.js) is the backstop. Latency-sensitive UI
// (chips, fog) stays immediate below — only the fetches wait out the window.
const refetchScheduled = {};
function coalesce(key, fn, ms = 2000) {
  if (refetchScheduled[key]) return;
  refetchScheduled[key] = true;
  setTimeout(() => { refetchScheduled[key] = false; fn(); }, ms);
}

// WS event kinds that mutate units/province/march/trade/messenger state — an open
// drawer showing that data should rebuild after them.
const DATA_KINDS = new Set([
  'ArmyArrival','BuildComplete','TrainComplete','MessengerArrival',
  'TradeCaravanArrival','UnitAttrition','UnitDeserted','UnitArrived','UnitExploreReturned',
]);

// fullResync refetches exactly what a fresh page load would — provinces, units,
// marches, messengers, trades, the fog/tile layer and the unread badge — so a
// reconnect or a tab-visible transition lands the client back on the truth.
export function fullResync() {
  const w = State.WORLD_ID;
  fetchAuth(`/api/v1/worlds/${w}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${w}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${w}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${w}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
  fetchAuth(`/api/v1/worlds/${w}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
  window.refreshTiles();
  fetchAuth(`/api/v1/worlds/${w}/notifications?unread=true`).then(r => r.ok && r.json().then(d => window.updateNotifBadge(d.unread || 0)));
  reloadDrawerDebounced(); // reflect the fresh data in an open drawer too
}

export function initWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  // BASE-prepend equivalent for the WS endpoint (see config.js): with BASE ''
  // (same origin, today) this is exactly the old `${proto}//${location.host}` URL.
  const wsBase = BASE ? BASE.replace(/^http/, 'ws') : `${proto}//${location.host}`;
  function connect() {
    ws = new WebSocket(`${wsBase}/ws/${State.WORLD_ID}`);
    ws.onopen = () => {
      State.lastWsMsgAt = Date.now();
      if (closedAt) { track('ws_reconnect', { downtime_s: Math.round((Date.now() - closedAt) / 1000) }); closedAt = null; }
      if (!firstConnect) {
        // Re-anchor the tick↔realtime mapping (Fas B): a reconnect is exactly
        // when the server may have restarted and paused the world, which is
        // what makes stored wall-clock stamps — and a stale anchor — lie.
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}`).then(r => r.ok && r.json().then(d => {
          if (d.current_tick != null && d.tick_seconds > 0) {
            State.CURRENT_TICK = d.current_tick; State.TICK_SECONDS = d.tick_seconds; State.TICK_ANCHOR_MS = serverNow();
          }
        }));
        fullResync(); // provinces + units + marches + messengers + trades + tiles + badge
      }
      firstConnect = false;
    };
    const PERSISTENT_KINDS = new Set([
      'BuildComplete','TrainComplete','ArmyArrival','ColonyFounded',
      'OutpostEstablished','OutpostCaptured','TradeDelivery','TradeLost','TradeReturn','MessengerArrival',
      'UnitAttrition','UnitDeserted','OfferAccepted','OfferDeclined','OfferExpired',
    ]);
    ws.onmessage = e => {
      State.lastWsMsgAt = Date.now();
      const msg = JSON.parse(e.data);
      if (['ArmyArrival','BuildComplete','TrainComplete'].includes(msg.kind)) {
        coalesce('provinces', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; window.MusicPlayer.update(); })));
        coalesce('marches', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; window.MusicPlayer.update(); })));
      }
      if (msg.kind === 'MessengerArrival') {
        coalesce('messengers', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; })));
        window.addNotifChip('diplomacy', '✉', msg.payload?.message || 'Messenger arrived', 'now');
      }
      if (msg.kind === 'ArmyArrival') {
        const intent = msg.payload?.intent || '';
        window.addNotifChip('war', '⚔', `Army arrived — ${intent}`, 'now');
      }
      if (msg.kind === 'BuildComplete') {
        window.addNotifChip('city', '🏛', `Build complete`, 'now');
      }
      if (msg.kind === 'MetropolisFounded') {
        window.addNotifChip('city', '👑', notifText('MetropolisFounded', msg.payload || {}), 'now');
      }
      if (msg.kind === 'ColonyFounded') {
        // Founding grain balance rides in the payload (DEL B) — a colony that
        // starts at a deficit drains its seed from THIS tick, so say so now.
        const p = msg.payload || {};
        const grainLine = colonyFoundedGrainLine(p);
        window.addNotifChip('city', '🏛', notifText('ColonyFounded', p) + (grainLine ? ' — ' + grainLine : ''), 'now');
      }
      if (msg.kind === 'TradeCaravanArrival') {
        coalesce('trades', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; })));
        window.addNotifChip('trade', '🐂', 'Caravan arrived', 'now');
      }
      if (msg.kind === 'KharisEvent') {
        window.addNotifChip('kult', '⛩', msg.payload?.message || 'Divine event', 'now');
      }
      if (msg.kind === 'UnitAttrition' || msg.kind === 'UnitDeserted') {
        // Units bleeding out from grain/silver shortage — previously silent.
        window.addNotifChip('war', notifIcon(msg.kind), notifText(msg.kind, msg.payload || {}), 'now');
        coalesce('units', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; })));
      }
      if (['OfferAccepted','OfferDeclined','OfferExpired'].includes(msg.kind)) {
        // Trade offer resolution — the offer's originator (see economy/trade.go,
        // messenger.go) — previously silent until the delayed TradeDelivery/
        // TradeReturn. Domain 'trade' → chip click opens the diplomacy drawer
        // via DOMAIN_DRAWER, where the offer thread actually lives.
        window.addNotifChip('trade', notifIcon(msg.kind), notifText(msg.kind, msg.payload || {}), 'now');
      }
      if (['UnitArrived','UnitExploreReturned','ArmyArrival'].includes(msg.kind)) {
        // A unit reached or left a hex: its route may have revealed fog and its
        // position changed. Refresh the fog map and the unit layer immediately
        // rather than waiting for the 30 s poll.
        window.refreshTiles();
        coalesce('units', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; })));
      }
      if (PERSISTENT_KINDS.has(msg.kind)) {
        coalesce('notifications', () => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/notifications?unread=true`)
          .then(r => r.ok && r.json().then(d => window.updateNotifBadge(d.unread || 0))));
      }
      // An open drawer showing units/province/trade data should follow the update.
      if (DATA_KINDS.has(msg.kind)) reloadDrawerDebounced();
    };
    ws.onclose = () => { closedAt = Date.now(); setTimeout(connect, 5000); };
  }
  connect();

  // Watchdog: a silently-dead path (NAT/WG drop with no FIN) leaves ws in OPEN
  // forever, so onclose never fires and reconnect never runs. Close it ourselves
  // once frames stop arriving past STALE_MS; onclose then schedules the retry.
  setInterval(checkWsLiveness, WATCHDOG_MS);
}
