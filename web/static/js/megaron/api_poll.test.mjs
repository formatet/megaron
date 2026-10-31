import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pollJSON } from './api.js';

function harness(t, responses) {
  const globals = Object.fromEntries(['document','localStorage','fetch'].map(k => [k,globalThis[k]]));
  t.after(() => { for (const [k,v] of Object.entries(globals)) v === undefined ? delete globalThis[k] : globalThis[k] = v; });
  const calls = [];
  globalThis.document = { getElementById: () => null };
  globalThis.localStorage = { getItem: () => 'token' };
  globalThis.fetch = async (url, opts) => { calls.push({url,opts}); return responses.shift(); };
  return calls;
}

test('a 304 poll returns the body the client already parsed, the same object', async t => {
  const calls = harness(t, [
    new Response('{"units":[1]}', {headers:{ETag:'W/"a"'}}),
    new Response(null, {status:304}),
    new Response('{"units":[2]}', {headers:{ETag:'W/"b"'}}),
  ]);
  const first = await pollJSON('/w/1/units');
  assert.deepEqual(first, {units:[1]});
  assert.equal(calls[0].opts.headers['If-None-Match'], undefined);
  assert.equal(await pollJSON('/w/1/units'), first, '304 hands back the held object');
  assert.equal(calls[1].opts.headers['If-None-Match'], 'W/"a"');
  assert.deepEqual(await pollJSON('/w/1/units'), {units:[2]});
});

test('a failed poll is undefined and ETags are held per URL', async t => {
  const calls = harness(t, [
    new Response('{}', {status:404}),
    new Response('[]', {headers:{ETag:'W/"x"'}}),
    new Response('[]'),
  ]);
  assert.equal(await pollJSON('/w/2/trades'), undefined);
  await pollJSON('/w/2/storms');
  await pollJSON('/w/3/storms');
  assert.equal(calls[2].opts.headers['If-None-Match'], undefined, 'another world asks unconditionally');
});

test('a changed server version offers a reload, the first one and repeats do not', async t => {
  harness(t, []);
  const notice = { classList: { added: [], add(c) { this.added.push(c); } } };
  globalThis.document = { getElementById: id => id === 'update-notice' ? notice : null };
  const { noteServerVersion } = await import('./api.js');
  noteServerVersion(null);
  noteServerVersion('abc.1');
  noteServerVersion('abc.1');
  assert.deepEqual(notice.classList.added, [], 'first seen and unchanged versions stay quiet');
  noteServerVersion('abc.2');
  assert.deepEqual(notice.classList.added, ['visible']);
});
