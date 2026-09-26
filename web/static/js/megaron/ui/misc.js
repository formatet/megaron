import { State, ownCapital } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';
import { setSoundMuted } from './sfx.js';

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
// and ws.js call MusicPlayer.update()/cue() from a lower layer that cannot
// import this module directly (config/state ← api/ws ← render ← ui ← main).
//
// Timothy 2026-09-25: music is a BED plus one-shot CUES, not modes — the old
// "war" mode (switching the loop itself based on State.marchData, which only
// ever populated from a recall) is gone. The bed is `<culture>_love`, looping
// (or a rotation — see BED_ROTATION below); `cue(name)` ducks it out, plays `<culture>_<name>` once, and fades
// the bed back in on `ended`. Routing from a WS kind to a cue name lives in
// sfx.js's musicCueFor (importable by ws.js without a cycle); the priority/
// throttle decision here is `shouldPlayCue`, kept pure for the same reason
// sfx.js's shouldPlay is: testable without an Audio element.
//
// Timothy 2026-09-26: for a culture listed in BED_ROTATION the bed is no longer
// one loop but a ROTATION of pieces that each have their own ending, with
// silence between them (megaron_ljud_minoisk_brief §0 — Colonization's tracks
// are never loops). Every other culture keeps its `<culture>_love` loop. The
// same piece may appear in two timbres (soundfont/synth) — variation without a
// new melody. The sign-in intro is deliberately NOT in the rotation.
const BED_ROTATION = {
  minoan: ['minoan_bygget_sf', 'minoan_bygget_synth'],
};
const BED_GAP_MIN_MS = 30 * 1000;
const BED_GAP_MAX_MS = 90 * 1000;

// nextBedTrack picks the next piece at random, never the one that just ended
// (with two tracks that is plain alternation). rand is injectable for tests.
export function nextBedTrack(list, last, rand = Math.random) {
  const pool = list.length > 1 ? list.filter((t) => t !== last) : list;
  return pool[Math.floor(rand() * pool.length)];
}

// bedGapMs — the silence after a piece ends, uniform in [30 s, 90 s].
export function bedGapMs(rand = Math.random) {
  return BED_GAP_MIN_MS + Math.floor(rand() * (BED_GAP_MAX_MS - BED_GAP_MIN_MS + 1));
}

const CUE_PRIORITY = { victory: 1, war: 2, doom: 3 };
const CUE_THROTTLE_MS = 10 * 60 * 1000; // a returning player must not get the
  // same cue three times for three sighted units in one sitting.

// shouldPlayCue is the whole decision: is sound on, has the player interacted
// yet, has this cue name been quiet long enough, and does it outrank whatever
// is already playing? lastPlayedAt is read, never mutated — the caller
// (MusicPlayer.cue) owns state and records the play only once this says yes.
export function shouldPlayCue(name, { now, muted, started, activeCue, lastPlayedAt }) {
  if (!started || muted) return false;
  const last = lastPlayedAt[name];
  if (last != null && now - last < CUE_THROTTLE_MS) return false;
  if (activeCue && CUE_PRIORITY[activeCue] >= CUE_PRIORITY[name]) return false;
  return true;
}

export const MusicPlayer = (() => {
  let cur = null;
  let curSrc = '';
  let curBed = '';        // culture whose bed (loop or rotation) is loaded
  let curTrack = '';      // rotation only: the piece in cur
  let inGap = false;      // rotation only: cur has ended, silence until gapTimer
  let gapTimer = null;
  let paused = false;
  let started = false;
  let activeCue = null;
  let activeCueAudio = null;
  const cueLastPlayedAt = Object.create(null);

  function tabHidden() {
    return typeof document !== 'undefined' && document.hidden;
  }

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
    if (started && !paused && !activeCue) {
      next.play().catch(() => {});
      ramp(next, 0.5, 1200);
    }
    if (cur) { const old = cur; ramp(old, 0, 800, () => old.pause()); }
    cur = next;
  }

  function canSound() {
    return started && !paused && !activeCue && !tabHidden();
  }

  // resume(ms) — every "bed comes back" path (start, unmute, cue ended, tab
  // visible) goes through here. Inside a rotation gap there is nothing to
  // resume: gapTimer brings the next piece in on its own.
  function resume(ms) {
    if (!cur || inGap) return;
    cur.play().catch(() => {});
    ramp(cur, 0.5, ms);
  }

  // Rotation: load `track` as cur; it plays now if it may, otherwise the next
  // resume() starts it. On `ended` (or a load error — audio must never throw
  // into the game) comes a silence, then the next piece.
  function playTrack(culture, track) {
    curTrack = track;
    inGap = false;
    const audio = new Audio('/static/music/' + track + '.ogg');
    audio.loop = false;
    audio.volume = 0;
    const done = () => {
      if (cur !== audio) return; // replaced (culture changed)
      inGap = true;
      gapTimer = setTimeout(() => {
        gapTimer = null;
        playTrack(culture, nextBedTrack(BED_ROTATION[culture], track));
      }, bedGapMs());
    };
    audio.addEventListener('ended', done);
    audio.addEventListener('error', done);
    if (cur) { const old = cur; ramp(old, 0, 800, () => old.pause()); }
    cur = audio;
    if (canSound()) { audio.play().catch(() => {}); ramp(audio, 0.5, 1200); }
  }

  function playBed(culture) {
    if (curBed === culture) return;
    curBed = culture;
    if (gapTimer) { clearTimeout(gapTimer); gapTimer = null; }
    const list = BED_ROTATION[culture];
    if (!list) { inGap = false; play('/static/music/' + culture + '_love.ogg'); return; }
    curSrc = '';
    playTrack(culture, nextBedTrack(list, ''));
  }

  function start() {
    if (started) return;
    started = true;
    // Intro tail pending (see playIntro() below): resume THAT, from wherever
    // its currentTime already sits, instead of starting the bed underneath
    // it — the bed only takes over once the intro's own finish() runs.
    if (activeCue === 'intro' && activeCueAudio) { activeCueAudio.play().catch(() => {}); return; }
    if (!paused) resume(1200);
  }

  function togglePause() {
    paused = !paused;
    if (paused) {
      if (cur) ramp(cur, 0, 500, () => cur.pause());
      // Muting silences a cue in progress too — one control for all audio.
      if (activeCueAudio) { activeCueAudio.pause(); activeCueAudio = null; activeCue = null; }
    } else {
      resume(500);
    }
    return paused;
  }

  // Init-time mute (persisted choice, applied before any user gesture) —
  // sets the flag directly, no ramping since nothing is playing yet.
  function setMuted(v) {
    paused = !!v;
  }

  // update() keeps the bed on the capital's culture. The war/love switch that
  // used to live here read State.marchData for an inbound attack — that table
  // is only ever populated by a recall (server/internal/messenger/recall.go),
  // so it practically never fired. War is now a cue, triggered off the real
  // ForeignMarchSighted/SettlementCaptured notifications (see cue() + ws.js).
  function update() {
    const capital = ownCapital();
    if (!capital || !capital.culture) return;
    playBed(capital.culture);
  }

  // cue(name) — a one-shot war/victory/doom sting. Ducks the bed out, plays
  // the cue once, and fades the bed back in when the cue ends (or errors —
  // audio must never throw into the game, so a load failure just skips
  // straight to "bed resumes").
  function cue(name) {
    // A hidden tab stays silent: the dispatch strip and SFX still carry the
    // news, and a cue here would end by restarting the bed in the background.
    if (tabHidden()) return;
    const now = Date.now();
    if (!shouldPlayCue(name, { now, muted: paused, started, activeCue, lastPlayedAt: cueLastPlayedAt })) return;
    const capital = ownCapital();
    if (!capital || !capital.culture) return;

    if (activeCueAudio) { activeCueAudio.pause(); activeCueAudio = null; } // outranked cue, cut short

    cueLastPlayedAt[name] = now;
    activeCue = name;

    if (cur) ramp(cur, 0, 800, () => cur.pause());

    const audio = new Audio('/static/music/' + capital.culture + '_' + name + '.ogg');
    audio.loop = false;
    audio.volume = 0;
    activeCueAudio = audio;
    audio.play().catch(() => {});
    ramp(audio, 0.6, 300);

    const finish = () => {
      if (activeCueAudio !== audio) return; // already superseded by a higher-priority cue
      activeCueAudio = null;
      activeCue = null;
      if (started && !paused && !tabHidden()) resume(800);
    };
    audio.addEventListener('ended', finish);
    audio.addEventListener('error', finish);
  }

  // Hidden tab: pause everything audible (fade the bed, drop any mid-play
  // cue outright — a cue quietly resuming out of context on return would be
  // stranger than just letting it not resolve). Visible again: only the bed
  // resumes, and only if the player hadn't muted it.
  function onHidden() {
    if (activeCueAudio) { activeCueAudio.pause(); activeCueAudio = null; activeCue = null; }
    if (cur) ramp(cur, 0, 500, () => cur.pause());
  }

  function onVisible() {
    if (started && !paused) resume(500);
  }

  // playIntro(pos) — sign-in → map handoff (Timothy 2026-09-25): the sign-in
  // screen's opening theme (index.html) keeps playing across the full-page
  // login navigation by handing its playback position through sessionStorage
  // (see introHandoff() below); this finishes that SAME track once, from
  // that position, before the normal bed takes over. Modelled on cue(): it
  // occupies activeCue/activeCueAudio exactly like a war/victory/doom cue,
  // so togglePause()/onHidden()/onVisible() and a real cue arriving mid-tail
  // already duck or drop it correctly with no changes there — shouldPlayCue
  // treats an unknown activeCue name ('intro' isn't in CUE_PRIORITY) as
  // outranked by everything, so any real cue is free to cut the tail short.
  // Two differences from cue(): there's no bed playing yet to duck out, and
  // finish() must actively start cur rather than calling play() again — a
  // second play() call with the same src is a silent no-op (see curSrc).
  function playIntro(pos) {
    if (paused) return;
    const audio = new Audio('/static/music/minoan_intro.ogg');
    audio.loop = false;
    try { audio.currentTime = pos; } catch (_) { /* metadata not loaded yet — applied once it is */ }
    audio.volume = 0.5;
    activeCue = 'intro';
    activeCueAudio = audio;
    // A successful immediate play (same-origin nav can carry over user
    // activation) counts as "started" the same way a real pointerdown does —
    // audio is already flowing. A rejection leaves started for start()'s
    // pointerdown fallback to set, resuming from the same currentTime.
    audio.play().then(() => { started = true; }).catch(() => {});

    const finish = () => {
      if (activeCueAudio !== audio) return; // superseded by a real cue
      activeCueAudio = null;
      activeCue = null;
      if (started && !paused && !tabHidden()) resume(800);
    };
    audio.addEventListener('ended', finish);
    audio.addEventListener('error', finish);
  }

  return { start, update, cue, togglePause, setMuted, onHidden, onVisible, playIntro };
})();

// One control for ALL audio: a player who silenced the music does not want a
// war horn either, and a second toggle would be a second thing to find. The
// choice is persisted (megaron_sound_muted) so it survives a reload.
const SOUND_MUTED_KEY = 'megaron_sound_muted';

export function toggleMusic() {
  const isPaused = MusicPlayer.togglePause();
  setSoundMuted(isPaused);
  try { localStorage.setItem(SOUND_MUTED_KEY, isPaused ? '1' : '0'); } catch (_) { /* no storage, no persistence */ }
  document.getElementById('music-btn').textContent = isPaused ? '♪' : '♫';
}

// Applies the persisted mute choice on load — before MusicPlayer.start() has
// ever run, so this only needs to set flags and the button glyph, never touch
// an Audio element. Missing/blocked storage reads back null → unmuted default.
export function initSoundPrefs() {
  let stored = null;
  try { stored = localStorage.getItem(SOUND_MUTED_KEY); } catch (_) { stored = null; }
  const muted = stored === '1';
  MusicPlayer.setMuted(muted);
  setSoundMuted(muted);
  const btn = document.getElementById('music-btn');
  if (btn) btn.textContent = muted ? '♪' : '♫';
}

// Autoplay policy: browsers require a user gesture before audio may play, so
// this listener IS the autoplay gate, not a detail — called from main.js as
// early as possible (see main.js's start() IIFE) to match the timing this
// used to have as a module-top-level statement, evaluated before bootstrap().
export function initMusicAutostart() {
  document.addEventListener('pointerdown', () => MusicPlayer.start(), { once: true });
}

// Hidden-tab handling — wired explicitly here (not a module-top-level
// listener) for the same reason initMusicAutostart() is: this file is
// imported by misc.test.mjs, which has no `document`, so any DOM wiring must
// stay inside a function the tests never call.
export function initMusicVisibility() {
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) MusicPlayer.onHidden();
    else MusicPlayer.onVisible();
  });
}

// ── Sign-in → map music handoff (Timothy 2026-09-25) ───────────────────────
// index.html (the sign-in screen, a plain server template with no ES
// modules) plays the opening theme and, right before the full-page redirect
// into the game, writes its playback position to this sessionStorage key so
// MusicPlayer.playIntro() can finish the SAME track here instead of
// restarting the bed cold. index.html cannot import this module, so the key
// name and the SOUND_MUTED_KEY-equivalent check there are duplicated
// literals — keep them in sync by hand if either changes.
const INTRO_HANDOFF_KEY = 'megaron_intro_handoff';
const INTRO_HANDOFF_MAX_AGE_MS = 120 * 1000; // a handoff from an abandoned
  // sign-in tab must not surface the intro minutes later on a fresh login.

// Pure decision, kept separate from the sessionStorage read so it's testable
// without a DOM: null for anything that isn't a trustworthy, fresh handoff
// (missing/unparseable JSON, a non-finite or negative pos, or `at` older
// than the age cap) — never a guess at 0 for a corrupt record.
export function introHandoff(raw, now) {
  if (!raw) return null;
  let parsed;
  try { parsed = JSON.parse(raw); } catch (_) { return null; }
  if (!parsed || typeof parsed !== 'object') return null;
  const { pos, at } = parsed;
  if (!Number.isFinite(pos) || pos < 0) return null;
  if (!Number.isFinite(at) || now - at > INTRO_HANDOFF_MAX_AGE_MS) return null;
  return { pos };
}

// Reads and removes the handoff key on map-page start-up. Called after
// initSoundPrefs() so MusicPlayer's mute flag already reflects the
// persisted ♫ choice — playIntro() itself is a no-op while muted.
export function initMusicIntroHandoff() {
  let raw = null;
  try {
    raw = sessionStorage.getItem(INTRO_HANDOFF_KEY);
    sessionStorage.removeItem(INTRO_HANDOFF_KEY);
  } catch (_) { /* no storage — the bed just starts cold, as before this feature */ }
  const handoff = introHandoff(raw, Date.now());
  if (handoff) MusicPlayer.playIntro(handoff.pos);
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

// "MonthName (N)" for the 12 ordinary months, so a Wanax can count days
// between two notifications without cross-referencing a name-only calendar
// (asynchronicity gate, megaron_arbetssatt.md). The intercalary Shadow Days
// (month 0) sit outside the numbered 1..12 cycle, so they keep just the name
// — a "(0)" would read as an ordinal that doesn't exist.
export function monthLabel(cal) {
  return cal.month === 0 ? cal.monthName : `${cal.monthName} (${cal.month})`;
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
