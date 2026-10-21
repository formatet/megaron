import test from 'node:test';
import assert from 'node:assert/strict';

globalThis.localStorage = { getItem: () => null };
globalThis.location = { pathname: '/play/world', search: '', origin: 'http://test' };
const badge = { textContent: '', style: {} };
globalThis.document = { getElementById: id => id === 'diplo-badge' ? badge : null };
const { State } = await import('../state.js');
const { updateDiplomacyBadge, readDiplomacyLetters } = await import('./diplomacy_reads.js');

test('read receipts clear only displayed letters, preserve arrivals and pending offers, and survive failures honestly', async () => {
  State.WORLD_ID = 'world';
  const incoming = [{ id: 'a', read_at: null, trade_offer: { status: 'pending' } }, { id: 'b', read_at: 'already' }];
  updateDiplomacyBadge(incoming);
  assert.equal(badge.textContent, '1');
  const requests = [];
  let finish;
  globalThis.fetch = (url, opts) => {
    requests.push([url, opts.method]);
    return new Promise(resolve => { finish = () => resolve(new Response('{}', { status: 200 })); });
  };
  const reading = readDiplomacyLetters(['a']);
  updateDiplomacyBadge([...incoming, { id: 'new', read_at: null }]);
  finish();
  assert.equal(await reading, true);
  assert.equal(badge.textContent, '1', 'new unseen letter must remain unread');
  assert.deepEqual(requests, [['/api/v1/worlds/world/messengers/a/read', 'POST']]);
  assert.equal(incoming[0].trade_offer.status, 'pending');
  globalThis.fetch = async () => new Response('{}', { status: 403 });
  assert.equal(await readDiplomacyLetters(['new']), false);
  assert.equal(badge.textContent, '1', 'failed receipt must not hide unread');
  globalThis.fetch = async () => new Response('{}', { status: 200 });
  assert.equal(await readDiplomacyLetters(['new']), true);
  assert.equal(badge.style.display, 'none');
});
