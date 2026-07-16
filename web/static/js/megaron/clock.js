// ── Server-anchored clock ──────────────────────────────────────────────────
// Every timestamp the API hands out (departs_at, arrives_at, complete_at, …)
// is server wall-clock time, but the map interpolates against the PLAYER's
// clock. Any skew between the two shifts every march/messenger/trade walker
// along its route by that amount — a 40 s fast client makes a spearman "jump"
// 6–7 hexes off the start line and makes short marches teleport outright
// (progress ≥ 1 before the first frame). Player clocks are never trustworthy,
// so the client measures the skew instead: api.js feeds every response's
// `Date` header into noteServerDate(), and time-sensitive code asks
// serverNow() instead of Date.now().
//
// Layer note: this module has no DOM/State/api deps (module-level offset
// only), so low-layer modules like ui/format.js may import it freely.

let offsetMs = 0; // server minus client; 0 until the first response lands

// The Date header has whole-second resolution, so consecutive responses
// naturally disagree by up to ~1 s. Only re-anchor when the measurement moves
// beyond that noise floor — a stable offset keeps walkers from micro-jumping
// every poll.
const NOISE_MS = 1500;

export function noteServerDate(dateHeader) {
  if (!dateHeader) return;
  const server = new Date(dateHeader).getTime();
  if (Number.isNaN(server)) return;
  // +500ms: the header truncates to the second; assume mid-second on average.
  const measured = server + 500 - Date.now();
  if (Math.abs(measured - offsetMs) > NOISE_MS) offsetMs = measured;
}

// Server wall-clock "now", in ms — use this instead of Date.now() whenever
// comparing against or interpolating between server timestamps.
export function serverNow() {
  return Date.now() + offsetMs;
}
