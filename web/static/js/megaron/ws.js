import { BASE } from './config.js';
import { State } from './state.js';
import { serverNow } from './clock.js';
import { fetchAuth } from './api.js';
import { notifText, notifIcon, colonyFoundedGrainLine } from './ui/format.js';

// ── WebSocket — real-time province updates ────────────────────────────────
// notifText/notifIcon/colonyFoundedGrainLine are pure formatting helpers
// (ui/format.js, no DOM/state deps of their own) so this module imports them
// directly. MusicPlayer, addNotifChip, refreshTiles and updateNotifBadge live
// in higher layers (ui/misc.js, ui/chips.js, render/map.js) that this module
// is not allowed to import per the config/state ← api/ws ← render ← ui ← main
// dependency order — those are reached via the window.* bridge that main.js
// sets up (same convention used for canvas → drawer calls in render/map.js).
export function initWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  // BASE-prepend equivalent for the WS endpoint (see config.js): with BASE ''
  // (same origin, today) this is exactly the old `${proto}//${location.host}` URL.
  const wsBase = BASE ? BASE.replace(/^http/, 'ws') : `${proto}//${location.host}`;
  let ws;
  let firstConnect = true;
  // Watchdog thresholds. The server pings + heartbeats every ~25 s (notify/hub.go);
  // if nothing at all arrives for STALE_MS the path is dead (silent NAT/WG drop that
  // never sends a FIN), so force-close to trigger the reconnect below.
  const STALE_MS = 65000;
  const WATCHDOG_MS = 20000;
  function connect() {
    ws = new WebSocket(`${wsBase}/ws/${State.WORLD_ID}`);
    ws.onopen = () => {
      State.lastWsMsgAt = Date.now();
      if (!firstConnect) {
        // Re-anchor the tick↔realtime mapping (Fas B): a reconnect is exactly
        // when the server may have restarted and paused the world, which is
        // what makes stored wall-clock stamps — and a stale anchor — lie.
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}`).then(r => r.ok && r.json().then(d => {
          if (d.current_tick != null && d.tick_seconds > 0) {
            State.CURRENT_TICK = d.current_tick; State.TICK_SECONDS = d.tick_seconds; State.TICK_ANCHOR_MS = serverNow();
          }
        }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
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
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; window.MusicPlayer.update(); }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; window.MusicPlayer.update(); }));
      }
      if (msg.kind === 'MessengerArrival') {
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
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
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
        window.addNotifChip('trade', '🐂', 'Caravan arrived', 'now');
      }
      if (msg.kind === 'KharisEvent') {
        window.addNotifChip('kult', '⛩', msg.payload?.message || 'Divine event', 'now');
      }
      if (msg.kind === 'UnitAttrition' || msg.kind === 'UnitDeserted') {
        // Units bleeding out from grain/silver shortage — previously silent.
        window.addNotifChip('war', notifIcon(msg.kind), notifText(msg.kind, msg.payload || {}), 'now');
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
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
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(r => r.ok && r.json().then(d => { State.unitsData = d.units || []; State.dirty = true; }));
      }
      if (PERSISTENT_KINDS.has(msg.kind)) {
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/notifications?unread=true`)
          .then(r => r.ok && r.json().then(d => window.updateNotifBadge(d.unread || 0)));
      }
    };
    ws.onclose = () => setTimeout(connect, 5000);
  }
  connect();

  // Watchdog: a silently-dead path (NAT/WG drop with no FIN) leaves ws in OPEN
  // forever, so onclose never fires and reconnect never runs. Close it ourselves
  // once frames stop arriving past STALE_MS; onclose then schedules the retry.
  setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN &&
        State.lastWsMsgAt && Date.now() - State.lastWsMsgAt > STALE_MS) {
      ws.close();
    }
  }, WATCHDOG_MS);
}
