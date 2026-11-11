import test from 'node:test';
import assert from 'node:assert/strict';
import { redirectBody, redirectPickedHTML, REDIRECT_NEEDS_BOTH } from './redirect_pick.js';
import { renderUnitCard } from './force_card.js';

test('redirect body is exactly what the old Send posted', () => {
  assert.deepEqual(redirectBody('12', '21'), { body: { target_q: 12, target_r: 21 } });
  assert.deepEqual(redirectBody('-3', '0'), { body: { target_q: -3, target_r: 0 } });
  assert.equal(JSON.stringify(redirectBody('4', '5').body), '{"target_q":4,"target_r":5}');
});

test('empty or non-numeric Q/R is refused with the sentence', () => {
  for (const [q, r] of [['', ''], ['1', ''], ['', '2'], [' ', '3'], [undefined, '1'], ['x', '1']])
    assert.deepEqual(redirectBody(q, r), { error: 'Type both Q and R.' }, JSON.stringify([q, r]));
  assert.equal(REDIRECT_NEEDS_BOTH, 'Type both Q and R.');
});

test('picked line carries the coordinates, Send order and Change', () => {
  const html = redirectPickedHTML('u1', 12, 21);
  assert.match(html, /Redirect to \(12,21\) — a messenger carries the order\./);
  assert.match(html, /unitRedirect\('u1'\)/);
  assert.match(html, /Send order →/);
  assert.match(html, /unitRedirectToggle\('u1'\)[^>]*>Change/);
});

test('the card no longer tells a phone player to right-click, and the typed inputs start empty', () => {
  const u = { id: 'u1', status: 'marching', category: 'land', type: 'spearman', display_name: 'Spearmen of Phaistos', target_q: 4, target_r: 2, quantity: 10 };
  const html = renderUnitCard(u);
  assert.doesNotMatch(html, /Right-click/);
  assert.match(html, /id="uredir-q-u1"[^>]*value=""/);
  assert.match(html, /id="uredir-r-u1"[^>]*value=""/);
  assert.match(html, /or type coordinates/);
  assert.match(html, /id="uredir-picked-u1"/);
});
