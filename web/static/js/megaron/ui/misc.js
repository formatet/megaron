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

// Autoplay policy: browsers require a user gesture before audio may play, so
// this listener IS the autoplay gate, not a detail — called from main.js as
// early as possible (see main.js's start() IIFE) to match the timing this
// used to have as a module-top-level statement, evaluated before bootstrap().
export function initMusicAutostart() {
  document.addEventListener('pointerdown', () => MusicPlayer.start(), { once: true });
}

// ── Celestial clock ───────────────────────────────────────────────────────
// Minoan calendar (Timothy 2026-08-06): month is 0..12, where 0 is the
// special case — the 5 intercalary Shadow Days of the Goddess outside any
// month. 1..12 are the ordinary 30-tick months; names are a pure lookup
// table (the "culture"), swappable without touching the month arithmetic.
// No Roman numerals, no real-world date — every figure here comes from
// world.current_tick, never the wall clock (only the animation frame
// between two ticks does).
const MONTH_NAMES = [
  'the Shadow Days of the Goddess', // month 0 — always the special case
  'the Pithoi', 'the Labyrinth', 'the Olive',
  'the Crocus', 'the Bull', 'the Labrys',
  'the Grain', 'the Ships', 'the Vine',
  'the Murex', 'the Depths', 'the Mists',
];
const DAYS_PER_MONTH        = 30;
const MONTHS_PER_YEAR       = 12;
const REGULAR_DAYS_PER_YEAR = DAYS_PER_MONTH * MONTHS_PER_YEAR; // 360
const DAYS_PER_YEAR         = REGULAR_DAYS_PER_YEAR + 5;        // 365, +5 Shadow Days

function monthOfYear(dayOfYear) {  // 0..12 — 0 = intercalary Shadow Days
  return dayOfYear < REGULAR_DAYS_PER_YEAR ? Math.floor(dayOfYear / DAYS_PER_MONTH) + 1 : 0;
}
function dayOfMonth(dayOfYear, month) {  // 1..30, or 1..5 during month 0
  return month === 0 ? dayOfYear - REGULAR_DAYS_PER_YEAR + 1 : (dayOfYear % DAYS_PER_MONTH) + 1;
}

// K4 tick anchor (State.CURRENT_TICK/TICK_SECONDS/TICK_ANCHOR_MS, re-anchored
// on WS reconnect — see state.js), continuous: null while unanchored.
function currentAbsoluteTick() {
  if (State.CURRENT_TICK == null || !State.TICK_SECONDS || State.TICK_ANCHOR_MS == null) return null;
  const elapsedTicks = (Date.now() - State.TICK_ANCHOR_MS) / (State.TICK_SECONDS * 1000);
  return State.CURRENT_TICK + elapsedTicks;
}

// Exact calendar reading (day/month/year) off the current tick — used by the
// Notifications drawer as well as the celestial widget below, so both read
// the same clock instead of two independent derivations drifting apart.
export function currentCalendarDate() {
  const absoluteTick = currentAbsoluteTick();
  if (absoluteTick == null) return null;
  const tick      = Math.floor(absoluteTick);
  const dayOfYear = tick % DAYS_PER_YEAR;
  const year      = Math.floor(tick / DAYS_PER_YEAR) + 1;
  const month     = monthOfYear(dayOfYear);
  const day       = dayOfMonth(dayOfYear, month);
  return { day, month, monthName: MONTH_NAMES[month], year };
}

function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

const SUN_R  = 8.5; // 70% bigger than the original 5/6 (Timothy 2026-08-06)
const MOON_R = 10.2;

function updateCelestial() {
  const absoluteTick = currentAbsoluteTick();
  if (absoluteTick == null) return;
  const tick        = Math.floor(absoluteTick);
  const phaseInTick = absoluteTick - tick; // 0..1 across the tick — one sun+moon lap

  const isDay = phaseInTick < 0.5;
  const localT = isDay ? phaseInTick / 0.5 : (phaseInTick - 0.5) / 0.5; // 0..1 within this half
  // Both halves sweep the same arc in the same direction — sun and moon rise
  // on the same side and set on the same side, like the real sky, instead of
  // the moon retracing the sun's path backwards.
  const angle = Math.PI - localT * Math.PI;

  const celBody   = document.getElementById('cel-body');
  const clipCirc  = document.getElementById('cel-body-clip-c');
  const shadow    = document.getElementById('cel-shadow');
  const nightTxt  = document.getElementById('cel-night-txt');
  const cx = 40, cy = 38, r = 34;
  const bodyX = cx + r * Math.cos(angle);
  const bodyY = cy - r * Math.sin(angle);
  celBody.setAttribute('cx', bodyX.toFixed(1));
  celBody.setAttribute('cy', bodyY.toFixed(1));
  clipCirc.setAttribute('cx', bodyX.toFixed(1));
  clipCirc.setAttribute('cy', bodyY.toFixed(1));

  const cal = currentCalendarDate(); // same tick, same math — see above

  if (isDay) {
    celBody.setAttribute('r', String(SUN_R));
    celBody.setAttribute('fill', cssVar('--gold'));
    clipCirc.setAttribute('r', String(SUN_R));
    shadow.setAttribute('display', 'none');
    nightTxt.setAttribute('display', 'none');
  } else {
    celBody.setAttribute('r', String(MOON_R));
    celBody.setAttribute('fill', cssVar('--moonlight'));
    clipCirc.setAttribute('r', String(MOON_R));
    nightTxt.setAttribute('display', '');

    // The moon's own disk carries its phase — a shadow circle of the same
    // radius, clipped to the disk, slid across it. Slide = 0 at new moon
    // (shadow fully covers the disk) and 2×MOON_R at full moon (shadow
    // clears the disk entirely). Continuous per-day, not a stepped lookup —
    // one true value per day of the month, not a handful of emoji buckets.
    // Shadow Days have no month position, so they're rendered as new moon.
    const monthPos    = cal.month === 0 ? 0 : (cal.day - 1) / DAYS_PER_MONTH; // 0..~0.97
    const litFraction = cal.month === 0 ? 0 : 1 - Math.abs(2 * monthPos - 1); // 0 new, 1 full
    const side        = monthPos < 0.5 ? 1 : -1; // waxing slides one way, waning the other
    const slide       = litFraction * 2 * MOON_R * side;
    shadow.setAttribute('cx', (bodyX + slide).toFixed(1));
    shadow.setAttribute('cy', bodyY.toFixed(1));
    shadow.setAttribute('r', String(MOON_R));
    shadow.setAttribute('fill', cssVar('--bg'));
    shadow.setAttribute('display', '');
  }

  // Just the month name (or the Shadow Days phrase for month 0, already
  // phrased whole in MONTH_NAMES[0]) — day and year moved to the
  // Notifications drawer's exact date line (currentCalendarDate() above).
  document.getElementById('cel-date').innerHTML = `<strong>${cal.monthName}</strong>`;
}

// Needs the K4 tick anchor (State.CURRENT_TICK/TICK_SECONDS/TICK_ANCHOR_MS),
// so main.js calls this only after bootstrap() has populated State.
export function initCelestial() {
  updateCelestial();
  // Repaint often enough for the arc to look continuous within one tick,
  // capped at the old 3 s cadence — a fixed 3 s would be half a game-day per
  // frame on a 6 s acceptance-world tick.
  const intervalMs = State.TICK_SECONDS
    ? Math.min(3000, State.TICK_SECONDS * 1000 / 20)
    : 3000;
  setInterval(updateCelestial, intervalMs);
}

// ── Locked-verb hints — server-authoritative (GET .../actions) ────────────
// Same source of truth as `keryx actions`: no client-side gate logic here,
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
