import { State, ownCapital } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';

// ── Lawagetas advisory voice ──────────────────────────────────────────────
const LAWAGETAS_BRIEFS = {
  city:      "Your megaron rises above all you rule, Wanax. Here your people labor — farmers, craftsmen, soldiers — each serving the palace that feeds and protects them. Assign your workers well; a foundry lets your smiths turn copper and tin into bronze. Idle hands do not fill the granary.",
  war:       "Bronze arms await your command, Wanax. March the army to distant lands — to raid, reinforce, or colonize. An order to recall or redirect a marching host travels by messenger, not by will alone; it takes time to arrive. In battle, numbers count, but walls and elite agema often decide the day.",
  diplomacy: "Words travel on foot, Wanax — a messenger's legs are your reach. You may only treat with those cities whose walls your messengers have touched. Consult the Cities ledger for rumours of distant lands, and the Rulers roll for who commands them; then send scouts, then make your offers — to buy or to sell.",
  economy:   "The palace scribes track every ingot and measure of grain, Wanax. Move goods freely between your own cities, but silver alone crosses borders with strangers. Watch the Wants ledger — it names what your trading partners hunger for, and where your surplus might fetch a premium.",
  kult:      "The gods watch your megaron, Wanax. Your temple's cult level and divine mood shape what rites you may call upon — tend it, and the gods answer; neglect it, and they turn away. Each prayer asks its own offering; choose with care.",
  notif:     "Your herald brings word from beyond the megaron walls, Wanax — arrivals, battles resolved, buildings completed. Matters that demand your attention rise here first.",
};

export function showLawagatasBrief(name) {
  const text = LAWAGETAS_BRIEFS[name];
  if (!text) return;
  if (sessionStorage.getItem('lb_dismissed_' + name)) return;
  const drawer = document.getElementById('drawer-' + name);
  if (!drawer || drawer.querySelector('.lawagetas-brief')) return;
  const el = document.createElement('div');
  el.className = 'lawagetas-brief';
  el.id = 'lb-' + name;
  el.innerHTML = '<div class="lb-head">⊛ Lawagetas</div>' + text +
    '<button class="lb-dismiss" onclick="dismissBrief(\'' + name + '\')" title="Dismiss">✕</button>';
  const header = drawer.querySelector('.drawer-header');
  if (header && header.nextSibling) {
    drawer.insertBefore(el, header.nextSibling);
  } else {
    drawer.appendChild(el);
  }
}

export function dismissBrief(name) {
  sessionStorage.setItem('lb_dismissed_' + name, '1');
  const el = document.getElementById('lb-' + name);
  if (el) el.remove();
}

// ── Music player ──────────────────────────────────────────────────────────
// Exposed on window (main.js: window.MusicPlayer = MusicPlayer) — render/map.js
// and ws.js call MusicPlayer.update() from a lower layer that cannot import
// this module directly (config/state ← api/ws ← render ← ui ← main).
export const MusicPlayer = (() => {
  let cur = null;
  let curSrc = '';
  let paused = false;
  let started = false;

  function ramp(el, to, ms, done) {
    const steps = 20, dt = ms / steps, dv = (to - el.volume) / steps;
    let i = 0;
    const t = setInterval(() => {
      el.volume = Math.max(0, Math.min(1, el.volume + dv));
      if (++i >= steps) { clearInterval(t); if (done) done(); }
    }, dt);
  }

  function play(src) {
    if (curSrc === src) return;
    curSrc = src;
    const next = new Audio(src);
    next.loop = true;
    next.volume = 0;
    if (started && !paused) {
      next.play().catch(() => {});
      ramp(next, 0.5, 1200);
    }
    if (cur) { const old = cur; ramp(old, 0, 800, () => old.pause()); }
    cur = next;
  }

  function start() {
    if (started) return;
    started = true;
    if (cur && !paused) { cur.play().catch(() => {}); ramp(cur, 0.5, 1200); }
  }

  function togglePause() {
    paused = !paused;
    if (paused) {
      if (cur) ramp(cur, 0, 500, () => cur.pause());
    } else {
      if (cur) { cur.play().catch(() => {}); ramp(cur, 0.5, 500); }
    }
    return paused;
  }

  function update() {
    const capital = ownCapital();
    if (!capital || !capital.culture) return;
    const ownSet = new Set(State.provinceData.filter(p => p.own).map(p => p.q + ',' + p.r));
    const war = State.marchData.some(m => m.intent === 'attack' && ownSet.has(m.target_q + ',' + m.target_r));
    play('/static/music/' + capital.culture + '_' + (war ? 'war' : 'love') + '.ogg');
  }

  return { start, update, togglePause };
})();

export function toggleMusic() {
  const isPaused = MusicPlayer.togglePause();
  document.getElementById('music-btn').textContent = isPaused ? '♪' : '♫';
}

document.addEventListener('pointerdown', () => MusicPlayer.start(), { once: true });

// ── Celestial clock ───────────────────────────────────────────────────────
const MONTH_NAMES = [
  'Posideon','Gamelion','Anthesterion','Elaphebolion',
  'Mounichion','Thargelion','Skirophorion','Hekatombaion',
  'Metageitnion','Boedromion','Pyanepsion','Maimakterion'
];
const MOON_PHASES = ['🌑','🌒','🌒','🌓','🌔','🌔','🌕','🌖','🌖','🌗','🌘','🌘'];
const REF_NEW_MOON = new Date('2025-01-29T12:35:00Z').getTime();
const LUNAR_CYCLE  = 29.53058867 * 24 * 3600 * 1000;

function moonPhase() {
  const age = ((Date.now() - REF_NEW_MOON) % LUNAR_CYCLE + LUNAR_CYCLE) % LUNAR_CYCLE;
  return MOON_PHASES[Math.floor(age / LUNAR_CYCLE * 12) % 12];
}

function toRoman(n) {
  const vals = [10,9,5,4,1], syms = ['X','IX','V','IV','I'];
  let r = '';
  for (let i = 0; i < vals.length; i++) { while (n >= vals[i]) { r += syms[i]; n -= vals[i]; } }
  return r;
}

function updateCelestial() {
  const now   = new Date();
  const epoch = State.WORLD_CREATED_AT ? new Date(State.WORLD_CREATED_AT) : new Date('2026-06-01T00:00:00Z');

  // Scaled game time: State.TIME_SCALE ms of game time pass per 1 ms of real time.
  // gameElapsedMs = wall-clock elapsed × State.TIME_SCALE
  const gameElapsedMs = (now - epoch) * State.TIME_SCALE;

  // Position within the current scaled game-day (24 game-hours)
  const msPerDay   = 24 * 3600 * 1000;
  const dayFrac    = (gameElapsedMs % msPerDay) / msPerDay; // 0..1 across one game day
  const gameHour   = dayFrac * 24; // 0..24 hours within game-day

  const isDay = gameHour >= 6 && gameHour < 18;

  const celBody  = document.getElementById('cel-body');
  const nightTxt = document.getElementById('cel-night-txt');
  const cx = 40, cy = 38, r = 34;

  let bodyX, bodyY;
  if (isDay) {
    const t = (gameHour - 6) / 12;          // 0..1 across day arc
    const angle = Math.PI - t * Math.PI;
    bodyX = cx + r * Math.cos(angle);
    bodyY = cy - r * Math.sin(angle);
    celBody.setAttribute('r', '5');
    celBody.setAttribute('fill', '#F9E79F');
    nightTxt.setAttribute('display', 'none');
  } else {
    const hn = gameHour >= 18 ? gameHour - 18 : gameHour + 6;
    const t  = hn / 12;                     // 0..1 across night arc
    const angle = t * Math.PI;
    bodyX = cx + r * Math.cos(angle);
    bodyY = cy - r * Math.sin(angle);
    celBody.setAttribute('r', '4');
    celBody.setAttribute('fill', '#D0D8F0');
    nightTxt.setAttribute('display', '');
  }

  celBody.setAttribute('cx', bodyX.toFixed(1));
  celBody.setAttribute('cy', bodyY.toFixed(1));
  document.getElementById('cel-phase').textContent = isDay ? '☀' : moonPhase();

  // Game date derived from scaled elapsed time
  const gameDaysSinceEpoch = Math.max(0, Math.floor(gameElapsedMs / msPerDay));
  const dayOfMonth = (gameDaysSinceEpoch % 30) + 1;
  const monthIdx   = Math.floor(gameDaysSinceEpoch / 30) % 12;
  const year       = Math.floor(gameDaysSinceEpoch / 360) + 1;

  document.getElementById('cel-date').innerHTML =
    `<strong>${toRoman(dayOfMonth)}</strong> ${MONTH_NAMES[monthIdx]}<br>Year ${toRoman(year)}`;
}

// Needs State.WORLD_CREATED_AT / State.TIME_SCALE, so main.js calls this only
// after bootstrap() has populated State (see main.js init order).
export function initCelestial() {
  updateCelestial();
  // Update every 3 seconds so movement is visible at State.TIME_SCALE=100 (day = 14.4 min IRL)
  setInterval(updateCelestial, 3 * 1000);
}

// ── Locked-verb hints — server-authoritative (GET .../actions) ────────────
// Same source of truth as `poleia actions`: no client-side gate logic here,
// just render what the server already decided (temenos_capabilities.md).
// Shared by every content drawer (city/war/economy/kult/diplomacy) — kept
// here rather than forced into one drawer's module ("Hellre en ärlig
// misc-modul än krystade hem" — the plan's own guidance for this file).
export async function renderLockedActions(category, provinceID) {
  const id = provinceID || (ownCapital() || {}).id;
  if (!id) return '';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${id}/actions`);
    if (!r.ok) return '';
    const verbs = (await r.json()).filter(v => v.category === category && !v.available);
    if (!verbs.length) return '';
    return '<div class="dsec"><div class="dsec-title" style="color:var(--text-dim)">Locked</div>' +
      verbs.map(v => {
        const hint = (v.requirements.find(req => !req.satisfied) || {}).hint || '';
        return '<div class="stat-row"><span class="sr-label">' + esc(v.name) + '</span>' +
          '<span class="sr-val" style="color:var(--text-dim);font-size:.7rem;text-align:right">' + esc(hint) + '</span></div>';
      }).join('') + '</div>';
  } catch (_) {
    return '';
  }
}
