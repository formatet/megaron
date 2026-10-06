import test from 'node:test';
import assert from 'node:assert/strict';
import { loadSearchMessages, searchMessagesHTML } from './search_messages.js';

const data = {
  inbox: [{ from_name: 'Knossos', to_name: 'Petras', message: 'We need copper <script>alert(1)</script>' }],
  gossip: [{ source_region: 'Northern coast', category: 'trade', text: 'Tin is scarce' }],
};
test('searches received letters by text and either city, case-insensitively', () => {
  for (const q of [' COPPER ', 'knossos', 'PETRAS']) {
    const html = searchMessagesHTML(q, data);
    assert.match(html, /data-search-drawer="diplomacy"/);
    assert.match(html, /Knossos/);
    assert.match(html, /&lt;script&gt;/);
    assert.doesNotMatch(html, /<script>/);
    assert.doesNotMatch(html, /Rumours/);
  }
});
test('searches rumours by text, region and category without exposing coordinates', () => {
  for (const q of ['tin', 'north', 'trade']) {
    const html = searchMessagesHTML(q, data);
    assert.match(html, /data-search-drawer="gossip"/);
    assert.match(html, /Tin is scarce/);
    assert.doesNotMatch(html, /centreOn|Received letters/);
  }
});
test('blank query and nonmatches add no rows; untrusted sender and region are escaped', () => {
  assert.equal(searchMessagesHTML('', data), '');
  assert.equal(searchMessagesHTML('absent', data), '');
  assert.doesNotMatch(searchMessagesHTML('hello', {
    inbox: [{ from_name: '<img onerror=evil>', message: 'hello' }],
    gossip: [{ source_region: '<svg onload=evil>', text: 'hello' }],
  }), /<(img|svg)/);
});
test('loads only current-world inbox and gossip using supplied authenticated reader', async () => {
  const urls = [];
  const result = await loadSearchMessages('world-1', async url => {
    urls.push(url);
    return { ok: true, json: async () => url.endsWith('/inbox') ? data.inbox : data.gossip };
  });
  assert.deepEqual(urls.sort(), ['/api/v1/worlds/world-1/gossip', '/api/v1/worlds/world-1/messengers/inbox']);
  assert.deepEqual(result, { ...data, failed: [] });
});
test('partial API failure preserves the other source and reports missing results', async () => {
  const result = await loadSearchMessages('world-1', async url => {
    if (url.endsWith('/inbox')) throw new Error('offline');
    return { ok: true, json: async () => data.gossip };
  });
  assert.deepEqual(result, { inbox: [], gossip: data.gossip, failed: ['Received letters'] });
});
test('HTTP failure or invalid response is not reported as a successful empty inbox', async () => {
  for (const response of [{ ok: false }, { ok: true, json: async () => ({ error: 'bad shape' }) }]) {
    const result = await loadSearchMessages('world-1', async () => response);
    assert.deepEqual(result.failed, ['Received letters', 'Rumours']);
  }
});
test('null SQL aggregate responses are valid empty results', async () => {
  const result = await loadSearchMessages('world-1', async () => ({ ok: true, json: async () => null }));
  assert.deepEqual(result, { inbox: [], gossip: [], failed: [] });
});
