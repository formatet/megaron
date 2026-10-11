import test from 'node:test';
import assert from 'node:assert/strict';
import { createChunkWorker } from './chunk_worker.js';
import { createMarkRenderer } from './markraster.js';

function rig() {
  const messages = [], received = []; let failures = 0, terminated = false;
  const worker = { postMessage: m => messages.push(m), terminate() { terminated = true; } };
  const client = createChunkWorker(() => worker, (...a) => received.push(a), () => failures++);
  worker.onmessage({ data: { type: 'ready' } });
  const job = (key, sig = 'known') => ({ ix: key, iy: 0, key, sig });
  const reply = (request, chunk = {}) => worker.onmessage({ data: { ...request, type: 'chunk', chunk, bakeMs: 3 } });
  return { worker, client, messages, received, job, reply, get failures() { return failures; }, get terminated() { return terminated; } };
}
test('worker queue keeps only one bake in flight and replaces obsolete camera priorities', () => {
  const r = rig(); r.client.setScene({ WORLD_ID: 'Mycenae' });
  r.client.request([r.job(1), r.job(2), r.job(3)]);
  const first = r.messages.at(-1);
  assert.equal(first.key, 1); assert.equal(r.client.pending, 3);
  r.client.request([r.job(1), r.job(9), r.job(2)]);
  assert.equal(r.messages.filter(m => m.type === 'bake').length, 1, 'one bake in flight');
  r.reply(first);
  assert.equal(r.messages.at(-1).key, 9, 'new viewport before the old prefetch ring');
  r.reply(r.messages.at(-1)); r.reply(r.messages.at(-1));
  assert.deepEqual(r.received.map(([j]) => j.key), [1, 9, 2]); assert.equal(r.client.pending, 0);
});
test('late river/FOW/world result is discarded and both transferred bitmaps are closed', () => {
  const r = rig(); r.client.setScene({ WORLD_ID: 'Mycenae', tileData: [{ terrain: 'river' }] });
  r.client.request([r.job(1)]); const old = r.messages.at(-1);
  r.client.setScene({ WORLD_ID: 'Tiryns', tileData: [{ terrain: 'fog' }] });
  r.client.request([r.job(1, 'fog')]);
  let closed = 0; r.reply(old, { img: { close() { closed++; } }, mem: { close() { closed++; } } });
  assert.equal(r.received.length, 0, 'stale scene must never enter the chunk cache');
  assert.equal(closed, 2, 'discarded image and memory mask must release resources');
  const next = r.messages.at(-1); assert.notEqual(next.generation, old.generation);
  r.reply(next); assert.equal(r.received[0][0].sig, 'fog');
});
test('worker failure terminates the queue and invokes the synchronous fallback once', () => {
  const r = rig(); r.client.setScene({}); r.client.request([r.job(1), r.job(2)]);
  r.worker.onerror(); r.worker.onmessage({ data: { type: 'failed' } });
  assert.equal(r.failures, 1); assert.equal(r.terminated, true); assert.equal(r.client.pending, 0);
  let closed = 0; r.reply(r.messages.at(-1), { img: { close() { closed++; } } });
  assert.equal(closed, 1, 'already posted result after failure is closed');
});
test('constructor failure allows immediate synchronous fallback', () => {
  assert.equal(createChunkWorker(() => { throw new Error('blocked worker'); }, () => {}, () => {}), null);
});
test('terrain renderer has independent scene caches and needs no DOM or global game State', () => {
  const a = { WORLD_ID: 'Mycenae', tileData: [{ q: 0, r: 0, terrain: 'river' }] };
  const b = { WORLD_ID: 'Tiryns', tileData: [{ q: 0, r: 0, terrain: 'fog' }] };
  const createCanvas = () => { throw new Error('geometry must not allocate a canvas'); };
  const left = createMarkRenderer(a, createCanvas), right = createMarkRenderer(b, createCanvas);
  assert.equal(left.terrainAt(0, 0), 'river'); assert.equal(right.terrainAt(0, 0), 'fog');
  a.tileData = [{ q: 0, r: 0, terrain: 'plains' }];
  assert.equal(left.terrainAt(0, 0), 'plains'); assert.equal(right.terrainAt(0, 0), 'fog');
  assert.notEqual(left.hash32(3, 5, 7), right.hash32(3, 5, 7), 'each world owns its own salt');
});
