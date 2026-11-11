import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { playerError, formatApiError, errorDetail } from './format.js';

const R = (status) => ({ status, ok: status < 400 });
const FRIENDLY = 'The server could not do that just now. Try again in a moment.';

// Rule 1
test('401 and the auth strings say logged out, never the raw string', () => {
  const out = 'You have been logged out. Log in again.';
  assert.equal(playerError(R(401), {}, 'x'), out);
  assert.equal(playerError(R(401), null, 'x'), out);
  assert.equal(playerError(R(400), { error: 'not authenticated' }, 'x'), out);
  assert.equal(playerError(null, { error: 'unauthorized' }, 'x'), out);
});
// Rule 2
test('technical strings and 5xx become the friendly line, raw kept as detail', () => {
  for (const e of ['invalid JSON', 'commit failed', 'transaction error', 'could not begin transaction',
    'invalid world ID', 'invalid province ID', 'invalid unit ID', 'could not load forces',
    'could not check hex', 'could not write the board', 'could not record gift dispatch', 'world not found']) {
    assert.equal(playerError(R(400), { error: e }, 'x'), FRIENDLY, e);
    assert.equal(errorDetail(R(400), { error: e }), e);
  }
  assert.equal(playerError(R(500), { error: 'anything at all' }, 'x'), FRIENDLY);
  assert.equal(errorDetail(R(500), { error: 'anything at all' }), 'anything at all');
});
// Rule 3
test('no JSON body or a network failure says no answer', () => {
  const out = 'No answer from the server. Check your connection and try again.';
  assert.equal(playerError(null, null, 'x', { network: true }), out);
  assert.equal(playerError(R(400), null, 'x', { network: true }), out);
});
// Rule 4
test('real rule refusals pass through unchanged', () => {
  for (const e of ['Not your settlement.', 'No citizens left in the pool.', 'Trade offer already accepted.']) {
    assert.equal(playerError(R(400), { error: e }, 'x'), e);
    assert.equal(errorDetail(R(400), { error: e }), '');
  }
});
test('lower-case sentence is capitalised and given a full stop', () => {
  assert.equal(playerError(R(409), { error: 'trade offer already accepted' }, 'x'), 'Trade offer already accepted.');
});
test('invalid credentials on a 401 is a rule refusal, not logged out', () => {
  assert.equal(playerError(R(401), { error: 'invalid credentials' }, 'x'), 'Invalid credentials.');
});
// Rule 5
test('fallback only when there is no message', () => {
  assert.equal(playerError(R(400), {}, 'Build failed.'), 'Build failed.');
  assert.equal(playerError(R(400), null, 'Build failed.'), 'Build failed.');
});
test('insufficient_goods and build_context keep their text', () => {
  const d = { error: 'insufficient_goods', missing: [{ good: 'timber', need: 39.9999, have: 12.2 }] };
  assert.equal(playerError(R(422), d, 'x'),
    'Not enough: timber 40 needed, 12 in store. Produce it here, or send it from another of your cities (Economy → Send goods).');
  assert.equal(formatApiError(d, 'x'), playerError(R(422), d, 'x'));
  assert.equal(playerError(R(409), { build_context: 1, message: 'Msg as is' }, 'x'), 'Msg as is');
});
test('formatApiError never prints a technical string', () => {
  assert.equal(formatApiError({ error: 'invalid JSON' }, 'x'), FRIENDLY);
});

// Guard (part B): a result element must not be set from .error without playerError.
function walk(dir, out = []) {
  for (const n of readdirSync(dir)) {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (n.endsWith('.js') && !n.endsWith('.test.js')) out.push(p);
  }
  return out;
}
test('guard: no result element is set directly from .error', () => {
  const root = join(dirname(fileURLToPath(import.meta.url)), '..');
  const bad = [];
  for (const f of walk(root)) {
    readFileSync(f, 'utf8').split('\n').forEach((line, i) => {
      if (/^\s*\/\//.test(line)) return;
      if (/\b(?:d|data|body|err|r|res|sdata|out)\.error\s*\|\|/.test(line) ||
          /(?:textContent|innerHTML)\s*=[^;]*\.error\b/.test(line) ||
          /showInlineResult\([^;]*\.error\b/.test(line)) bad.push(`${f}:${i + 1}: ${line.trim()}`);
    });
  }
  assert.deepEqual(bad, []);
});
