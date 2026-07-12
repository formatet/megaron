import { BASE } from './config.js';
import { State } from './state.js';
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
  function connect() {
    ws = new WebSocket(`${wsBase}/ws/${State.WORLD_ID}`);
    ws.onopen = () => {
      if (!firstConnect) {
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces`).then(r => r.ok && r.json().then(d => { State.provinceData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/marches`).then(r => r.ok && r.json().then(d => { State.marchData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
        fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`).then(r => r.ok && r.json().then(d => { State.tradeData = d; State.dirty = true; }));
      }
      firstConnect = false;
    };
    const PERSISTENT_KINDS = new Set([
      'BuildComplete','TrainComplete','ArmyArrival','ColonyFounded',
      'OutpostEstablished','OutpostCaptured','TradeDelivery','TradeLost','TradeReturn','MessengerArrival',
      'UnitAttrition','UnitDeserted',
    ]);
    ws.onmessage = e => {
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
}
