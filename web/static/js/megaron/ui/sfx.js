// ── War sounds ──────────────────────────────────────────────────────────────
//
// Timothy 2026-09-10: "göra nån slags krigsljud när soldater mottar marschorder
// - om man är inloggad när de får den - och ett krigsljud när de strider".
//
// Synthesised with WebAudio rather than shipped as files. Two reasons: there is
// no sound-effect asset pipeline (only /static/music/*.ogg loops, which are long
// culture beds, not one-shots), and a bronze-age horn is a handful of numbers —
// a low sawtooth with a bend on it. No asset means nothing to cache-bust and
// nothing to keep in sync with the culture set.
//
// Muting rides on the EXISTING ♫ button (ui/misc.js toggleMusic). One control
// for all audio: a player who silenced the music does not want a war horn, and
// a second toggle would be a second thing to find.
//
// Autoplay policy: browsers refuse audio until a user gesture. The context is
// created lazily on the first sound and resumed if suspended, by which point
// the player has clicked something (ordering a march IS a click). A battle
// sound arriving passively on a never-touched tab simply stays silent, which
// is the correct behaviour rather than a bug to work around.

let muted = false;
let ctx = null;

// Throttle per sound kind. A battle resolves into one notification per role,
// but nothing guarantees two battles cannot conclude in the same tick, and a
// stack of simultaneous horns is noise, not information.
const MIN_GAP_MS = 1500;
const lastPlayed = Object.create(null);

export function setSoundMuted(v) {
  muted = !!v;
}

export function isSoundMuted() {
  return muted;
}

// shouldPlay is the whole decision, kept pure so it can be tested without an
// audio device: is sound on, and has this kind been quiet long enough?
// Records the time when it says yes, so callers cannot double-fire.
export function shouldPlay(kind, now = Date.now()) {
  if (muted) return false;
  const last = lastPlayed[kind];
  if (last != null && now - last < MIN_GAP_MS) return false;
  lastPlayed[kind] = now;
  return true;
}

export function _resetSfx() {
  muted = false;
  for (const k of Object.keys(lastPlayed)) delete lastPlayed[k];
}

// audioCtx returns a running AudioContext, or null where audio is unavailable
// (no WebAudio, or a context the browser refuses to resume). Every caller below
// treats null as "stay silent" — sound is decoration, and decoration must never
// throw into the game loop.
function audioCtx() {
  try {
    const Ctor = globalThis.AudioContext || globalThis.webkitAudioContext;
    if (!Ctor) return null;
    if (!ctx) ctx = new Ctor();
    if (ctx.state === 'suspended') ctx.resume().catch(() => {});
    return ctx;
  } catch (_) {
    return null;
  }
}

function tone(ac, { type, from, to, peak, attack, duration, delay = 0, filterHz }) {
  const t = ac.currentTime + delay;
  const osc = ac.createOscillator();
  osc.type = type;
  osc.frequency.setValueAtTime(from, t);
  if (to && to !== from) osc.frequency.exponentialRampToValueAtTime(to, t + duration * 0.6);

  const g = ac.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(peak, t + attack);
  g.gain.exponentialRampToValueAtTime(0.0001, t + duration);

  let tail = g;
  if (filterHz) {
    const f = ac.createBiquadFilter();
    f.type = 'lowpass';
    f.frequency.setValueAtTime(filterHz, t);
    g.connect(f);
    tail = f;
  }
  tail.connect(ac.destination);
  osc.connect(g);
  osc.start(t);
  osc.stop(t + duration + 0.05);
}

function noiseBurst(ac, { peak, duration, centreHz, delay = 0 }) {
  const t = ac.currentTime + delay;
  const frames = Math.max(1, Math.floor(ac.sampleRate * duration));
  const buf = ac.createBuffer(1, frames, ac.sampleRate);
  const data = buf.getChannelData(0);
  for (let i = 0; i < frames; i++) {
    // Decaying white noise — the metal-on-metal part of a clash.
    data[i] = (Math.random() * 2 - 1) * (1 - i / frames);
  }
  const src = ac.createBufferSource();
  src.buffer = buf;

  const f = ac.createBiquadFilter();
  f.type = 'bandpass';
  f.frequency.setValueAtTime(centreHz, t);
  f.Q.setValueAtTime(0.8, t);

  const g = ac.createGain();
  g.gain.setValueAtTime(peak, t);
  g.gain.exponentialRampToValueAtTime(0.0001, t + duration);

  src.connect(f); f.connect(g); g.connect(ac.destination);
  src.start(t);
  src.stop(t + duration + 0.05);
}

// A horn call: two notes, low and open, the second a fifth above the first.
// Sawtooth through a lowpass reads as brass/bronze rather than as a synth beep.
export function playWarHorn() {
  if (!shouldPlay('horn')) return;
  const ac = audioCtx();
  if (!ac) return;
  tone(ac, { type: 'sawtooth', from: 146.8, to: 164.8, peak: 0.22, attack: 0.05, duration: 0.42, filterHz: 1100 });
  tone(ac, { type: 'sawtooth', from: 220.0, to: 246.9, peak: 0.20, attack: 0.05, duration: 0.55, delay: 0.30, filterHz: 1300 });
  // A soft octave-below body so the call has weight on small speakers.
  tone(ac, { type: 'triangle', from: 73.4, to: 82.4, peak: 0.14, attack: 0.06, duration: 0.85, filterHz: 700 });
}

// A clash: a drum thud under two noise bursts and a pair of metallic strikes.
export function playBattleClash() {
  if (!shouldPlay('battle')) return;
  const ac = audioCtx();
  if (!ac) return;
  tone(ac, { type: 'sine', from: 92, to: 55, peak: 0.30, attack: 0.005, duration: 0.28 });
  noiseBurst(ac, { peak: 0.20, duration: 0.30, centreHz: 2600 });
  noiseBurst(ac, { peak: 0.14, duration: 0.24, centreHz: 3400, delay: 0.16 });
  tone(ac, { type: 'square', from: 1180, to: 900, peak: 0.06, attack: 0.004, duration: 0.13, delay: 0.05, filterHz: 4000 });
  tone(ac, { type: 'square', from: 1560, to: 1200, peak: 0.05, attack: 0.004, duration: 0.11, delay: 0.22, filterHz: 4000 });
  tone(ac, { type: 'triangle', from: 78, to: 48, peak: 0.18, attack: 0.01, duration: 0.45, delay: 0.18 });
}

// ── Arrival and dispatch sounds (Timothy 2026-09-25) ────────────────────────
// "jag älskar bitmusikljudet som kommer när man ger order.. kan vi få ett
// liknande när de anländer? eller vid varje dispatch som kommer?"
//
// Both, but not the same sound: an arrival is an event the player caused and
// waited for, a dispatch is the world knocking. Only LIVE pushes play (ws.js);
// the strip rebuilt from the archive at login stays silent, or a returning
// Wanax would be greeted by a dozen chimes at once.

// Kinds that already carry their own sound in ws.js (horn, clash) — the chime
// must not stack on top of them.
const OWN_SOUND_KINDS = new Set(['BattleWon', 'BattleLost', 'UnitRecalled', 'UnitRedirected']);
const ARRIVAL_KINDS = new Set(['UnitArrived', 'UnitExploreReturned', 'ArmyArrival', 'MessengerReturned']);

// soundForKind is the whole routing decision, pure for testing:
// 'arrival' | 'chime' | null (null = the kind has its own sound, or none).
export function soundForKind(kind) {
  if (!kind || OWN_SOUND_KINDS.has(kind)) return null;
  return ARRIVAL_KINDS.has(kind) ? 'arrival' : 'chime';
}

// An arrival: the Minoan motif's neighbour figure A–B♭–A resolving up to D,
// over a soft drum tap — the horn's answer ("we are here"), in the same bronze
// voice. Timothy 2026-09-25 chose it by ear over the old D–F♯–A (a D-major
// fanfare that belonged to no mode the music uses); Phrygian, like the music.
export function playArrival() {
  if (!shouldPlay('arrival')) return;
  const ac = audioCtx();
  if (!ac) return;
  tone(ac, { type: 'sawtooth', from: 220.0, to: 220.0, peak: 0.13, attack: 0.02, duration: 0.16, filterHz: 1500 });
  tone(ac, { type: 'sawtooth', from: 233.1, to: 233.1, peak: 0.13, attack: 0.02, duration: 0.14, delay: 0.13, filterHz: 1600 });
  tone(ac, { type: 'sawtooth', from: 220.0, to: 220.0, peak: 0.13, attack: 0.02, duration: 0.16, delay: 0.24, filterHz: 1600 });
  tone(ac, { type: 'sawtooth', from: 293.7, to: 293.7, peak: 0.15, attack: 0.02, duration: 0.55, delay: 0.38, filterHz: 1800 });
  tone(ac, { type: 'sine', from: 110, to: 70, peak: 0.16, attack: 0.005, duration: 0.20, delay: 0.38 });
}

// A dispatch: one soft bell-like note with its fifth — quiet enough to hear
// many times an evening without wearing thin.
export function playDispatchChime() {
  if (!shouldPlay('chime')) return;
  const ac = audioCtx();
  if (!ac) return;
  tone(ac, { type: 'triangle', from: 659.3, to: 659.3, peak: 0.07, attack: 0.01, duration: 0.55 });
  tone(ac, { type: 'sine', from: 988.0, to: 988.0, peak: 0.035, attack: 0.01, duration: 0.40, delay: 0.02 });
}

// playForKind — the one call ws.js makes for every live dispatch.
export function playForKind(kind) {
  const s = soundForKind(kind);
  if (s === 'arrival') playArrival();
  else if (s === 'chime') playDispatchChime();
}

// ── Music cues (Timothy 2026-09-25) ─────────────────────────────────────────
// musicCueFor routes a live WS notification to a music cue name — pure, so it
// can be tested without an Audio element, and living here (not in ui/misc.js,
// which ws.js cannot import — see ws.js's header comment) so ws.js can call it
// directly. ws.js calls window.MusicPlayer.cue(cue) for a non-null result, in
// the same LIVE-push branch as playForKind above — never from the archive
// rebuild at login.
export function musicCueFor(kind, payload) {
  const p = payload || {};
  if (kind === 'ForeignMarchSighted' || kind === 'ForeignMarchSightedV2') {
    // threatens_settlement_id is only set when the march targets one of the
    // player's own settlements (combat/march_sighting.go) — a march sighted
    // elsewhere on the map is not this player's war.
    return p.threatens_settlement_id != null ? 'war' : null;
  }
  if (kind === 'SettlementCaptured' || kind === 'CityOccupied') {
    if (p.role === 'attacker') return 'victory';
    if (p.role === 'defender') return 'doom';
    return null;
  }
  return null;
}
