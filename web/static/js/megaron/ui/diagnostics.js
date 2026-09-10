// ── Diagnostics attached to a player's bug report ───────────────────────────
//
// Written for the reader, not the writer. Reading the 2026-09-04 and 09-09
// report sweeps, three questions came up again and again that the report itself
// could never answer, and each cost a round trip to Timothy or an unprovable
// guess:
//
//   "Which browser and how big a window?"  — reports 3d82447a and c12c29be were
//      Chromium-specific layout bugs (text too small, a tab that shrank the city
//      picture, the gossip tab riding over the map). The client knew the answer
//      the whole time; the report carried prose instead.
//   "What did the server actually refuse?" — "I clicked and nothing happened" is
//      the most common shape a report takes. The refusal text is the diagnosis.
//   "Did anything throw?" — a JS exception that kills a drawer reaches the player
//      as "the page is broken" and reaches the report as nothing at all.
//
// So: two small ring buffers plus an environment snapshot, folded into the
// report's existing free-form `context` blob. No migration — the column is
// JSONB and the server stores whatever the client sends (api/handlers/reports.go
// keeps it as json.RawMessage), which is exactly the extensibility mig 123 was
// designed for.
//
// Deliberately bounded: the last few entries only, every string truncated. A
// report is a lead, not a log — anything longer belongs in the server's own
// journal, and an unbounded buffer in a tab left open for nine hours (which is
// this game's NORMAL session shape) would grow without limit.

const MAX_ENTRIES = 5;
const MAX_TEXT = 200;

const apiFailures = [];
const jsErrors = [];

// Keep the newest MAX_ENTRIES, dropping the oldest — the failure that made the
// player file the report is the most recent one, so the tail is what matters.
function push(buffer, entry) {
  buffer.push(entry);
  while (buffer.length > MAX_ENTRIES) buffer.shift();
}

export function truncate(s, limit = MAX_TEXT) {
  const str = String(s ?? '');
  return str.length > limit ? str.slice(0, limit) + '…' : str;
}

// recordApiFailure is called from api.js for every non-ok response — the one
// choke point every API call already passes through.
export function recordApiFailure({ path, status, body, at }) {
  push(apiFailures, {
    path: truncate(path, 120),
    status,
    body: truncate(body),
    at: at || new Date().toISOString(),
  });
}

export function recordJsError({ message, source, at }) {
  push(jsErrors, {
    message: truncate(message),
    source: truncate(source, 120),
    at: at || new Date().toISOString(),
  });
}

// clientEnvironment answers the browser/window question outright. devicePixelRatio
// and the viewport together explain the "text is too small" class of report,
// which is otherwise indistinguishable from a CSS bug.
export function clientEnvironment(win = globalThis) {
  const nav = win.navigator || {};
  const env = {
    ua: truncate(nav.userAgent, 200),
    viewport: `${win.innerWidth || 0}x${win.innerHeight || 0}`,
    dpr: win.devicePixelRatio || 1,
  };
  if (nav.language) env.lang = nav.language;
  return env;
}

// diagnosticsSnapshot returns undefined when there is nothing to say, so the
// report's context stays absent rather than carrying empty scaffolding.
export function diagnosticsSnapshot(win = globalThis) {
  const snap = { client: clientEnvironment(win) };
  if (apiFailures.length) snap.recent_api_failures = apiFailures.slice();
  if (jsErrors.length) snap.recent_js_errors = jsErrors.slice();
  return snap;
}

// installErrorCapture wires the two global error channels. Both are passive
// listeners: nothing is swallowed, the browser still logs to the console as
// usual, and a failure inside the listener must never break the page that is
// already having a bad day — hence the try/catch.
export function installErrorCapture(win = globalThis) {
  if (!win || typeof win.addEventListener !== 'function') return;
  win.addEventListener('error', (e) => {
    try {
      recordJsError({
        message: (e && (e.message || (e.error && e.error.message))) || 'script error',
        source: e && e.filename ? `${e.filename}:${e.lineno || 0}` : '',
      });
    } catch (_) { /* never let diagnostics break the page */ }
  });
  win.addEventListener('unhandledrejection', (e) => {
    try {
      const reason = e && e.reason;
      recordJsError({
        message: 'unhandled rejection: ' + ((reason && reason.message) || reason || 'unknown'),
        source: '',
      });
    } catch (_) { /* never let diagnostics break the page */ }
  });
}

// Test seam only.
export function _resetDiagnostics() {
  apiFailures.length = 0;
  jsErrors.length = 0;
}
