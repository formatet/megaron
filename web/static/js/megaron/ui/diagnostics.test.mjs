import test from 'node:test';
import assert from 'node:assert/strict';

const {
  recordApiFailure, recordJsError, diagnosticsSnapshot, clientEnvironment,
  installErrorCapture, truncate, _resetDiagnostics,
} = await import('./diagnostics.js');

const fakeWin = (over = {}) => ({
  navigator: { userAgent: 'Mozilla/5.0 (X11; Linux x86_64) Chrome/140', language: 'sv-SE' },
  innerWidth: 1920, innerHeight: 1080, devicePixelRatio: 2,
  ...over,
});

test('DG1: the environment answers the question two Chromium reports could not', () => {
  // Reports 3d82447a / c12c29be were Chromium-specific layout bugs; the client
  // knew the browser and the window size the whole time.
  const env = clientEnvironment(fakeWin());
  assert.match(env.ua, /Chrome/);
  assert.equal(env.viewport, '1920x1080');
  assert.equal(env.dpr, 2, 'devicePixelRatio is what separates "text too small" from a CSS bug');
  assert.equal(env.lang, 'sv-SE');
});

test('DG2: a refused call is remembered with the server\'s own words', () => {
  _resetDiagnostics();
  recordApiFailure({
    path: '/api/v1/worlds/:id/provinces/:id/placements',
    status: 409,
    body: '{"error":"no citizens left in the pool — every citizen is already placed"}',
  });
  const snap = diagnosticsSnapshot(fakeWin());
  assert.equal(snap.recent_api_failures.length, 1);
  assert.equal(snap.recent_api_failures[0].status, 409);
  assert.match(snap.recent_api_failures[0].body, /no citizens left/);
  assert.match(snap.recent_api_failures[0].path, /placements/);
  assert.ok(snap.recent_api_failures[0].at, 'a timestamp so the reader can order events');
});

test('DG3: the buffer keeps the NEWEST entries — the failure that prompted the report is the last one', () => {
  _resetDiagnostics();
  for (let i = 1; i <= 8; i++) recordApiFailure({ path: '/p', status: 400 + i, body: 'e' + i });
  const failures = diagnosticsSnapshot(fakeWin()).recent_api_failures;
  assert.equal(failures.length, 5, 'bounded — a tab open for nine hours must not grow without limit');
  assert.equal(failures[failures.length - 1].body, 'e8', 'newest kept');
  assert.equal(failures[0].body, 'e4', 'oldest dropped');
});

test('DG4: long strings are truncated so a report stays a lead, not a log', () => {
  _resetDiagnostics();
  recordApiFailure({ path: '/p', status: 500, body: 'x'.repeat(5000) });
  const body = diagnosticsSnapshot(fakeWin()).recent_api_failures[0].body;
  assert.ok(body.length < 250, `expected a truncated body, got ${body.length} chars`);
  assert.match(body, /…$/);
});

test('DG5: with nothing wrong, the snapshot carries the environment and no empty scaffolding', () => {
  _resetDiagnostics();
  const snap = diagnosticsSnapshot(fakeWin());
  assert.ok(snap.client);
  assert.equal(snap.recent_api_failures, undefined);
  assert.equal(snap.recent_js_errors, undefined);
});

test('DG6: a thrown script error is captured with its source location', () => {
  _resetDiagnostics();
  const handlers = {};
  installErrorCapture({ addEventListener: (k, fn) => { handlers[k] = fn; } });
  handlers.error({ message: 'x is not a function', filename: '/static/js/megaron/ui/drawers/city.js', lineno: 455 });
  const errs = diagnosticsSnapshot(fakeWin()).recent_js_errors;
  assert.equal(errs.length, 1);
  assert.match(errs[0].message, /not a function/);
  assert.match(errs[0].source, /city\.js:455/);
});

test('DG7: an unhandled promise rejection is captured too — the half that never reaches window.onerror', () => {
  _resetDiagnostics();
  const handlers = {};
  installErrorCapture({ addEventListener: (k, fn) => { handlers[k] = fn; } });
  handlers.unhandledrejection({ reason: new Error('placement-options 500') });
  const errs = diagnosticsSnapshot(fakeWin()).recent_js_errors;
  assert.match(errs[0].message, /unhandled rejection: placement-options 500/);
});

test('DG8: a listener that throws never breaks the page it is diagnosing', () => {
  _resetDiagnostics();
  const handlers = {};
  installErrorCapture({ addEventListener: (k, fn) => { handlers[k] = fn; } });
  // A getter that throws is the realistic hostile shape (a proxied error object).
  const hostile = { get message() { throw new Error('boom'); } };
  assert.doesNotThrow(() => handlers.error(hostile));
});

test('DG9: installErrorCapture on a host without addEventListener is a no-op, not a crash', () => {
  assert.doesNotThrow(() => installErrorCapture(undefined));
  assert.doesNotThrow(() => installErrorCapture({}));
});

test('DG10: truncate leaves short strings untouched and handles null', () => {
  assert.equal(truncate('short'), 'short');
  assert.equal(truncate(null), '');
});
