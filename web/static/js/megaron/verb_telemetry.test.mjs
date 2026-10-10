import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { fetchAuth } from './api.js';
import { VERB_EVENTS, verbForRequest } from './verb_telemetry.js';

const root = new URL('../../../../', import.meta.url);
const snapshot = readFileSync(new URL('docs/reviews/umami-verb/verblista.md', root), 'utf8');
const vaultPath = process.env.MEGARON_VERBLISTA || `${homedir()}/Dokument/myltavault/megaron_verblista.md`;
const vault = existsSync(vaultPath) ? readFileSync(vaultPath, 'utf8') : snapshot;

// An explicit interpretation of compound rows, not a list inferred from the
// implementation under test. A new vault row or missing branch must fail.
const coverage = {
  'build': ['build'], 'cancel-build': ['cancel-build'], 'recruit': ['recruit'],
  'place / staff': ['place', 'staff', 'unplace'],
  'place → ta hex (gren av `place`)': ['place'], 'labor (cult)': ['labor'],
  'slaughter': ['slaughter'], 'disband': ['disband'], 'transfer': ['transfer', 'gift/tribute'],
  'abandon': ['abandon'], 'rite': ['rite'], 'gift': ['gift'], 'password': ['password'],
  'agora password': ['agora password'], 'occupation-order': ['occupation-order'],
  'return-army': [], // The vault explicitly excludes gated kingdom verbs.
  'march': ['march'], 'recall / redirect': ['recall', 'redirect'], 'stance': ['stance'],
  'retreat-order': ['retreat-order'], 'retreat-default': ['retreat-default'],
  'reinforce': ['reinforce'], 'repair': ['repair'], 'load / unload': ['load', 'unload'],
  'join': ['join'], 'rise / leave': ['rise', 'leave'], 'founding settle': ['founding settle'], 'message / reply': ['message', 'reply'],
  'trade offer/accept/decline/cancel': ['trade-offer', 'trade-accept', 'trade-decline', 'trade-cancel'],
  'arrange passage (skepp för ett väntande bud)': ['arrange passage'],
  'fetch by ship (hämta hem en fältenhet)': ['fetch by ship'],
  'call back (väntande bud i egen hamn)': ['call back'],
  '**standing order**': ['standing order', 'standing order pause', 'standing order resume', 'standing order delete'],
  'report': ['report'], 'notification-preferences': ['notification-preferences'],
  'mark notifications read (all)': ['mark notifications read (all)'],
  'mark notification read (one)': ['mark notification read (one)'],
};

function vaultRows(text) {
  const section = text.split('## Muterande verb')[1]?.split('## Läsytor')[0];
  assert.ok(section, 'vault mutation table is missing');
  return section.split('\n').filter(line => line.startsWith('| ') && !line.startsWith('| Verb') && !line.startsWith('|---'))
    .map(line => line.split('|')[1].trim());
}
function checkParity(text, catalog = VERB_EVENTS) {
  for (const label of vaultRows(text)) {
    assert.ok(Object.hasOwn(coverage, label), `Unmapped vault verb: ${label}`);
    for (const verb of coverage[label]) {
      assert.ok(catalog.some(entry => entry.verb === verb && entry.event), `No event for vault verb: ${label} (${verb})`);
    }
  }
}

test('Umami parity: every mutating vault verb has an event or explicit kingdom exclusion', () => {
  checkParity(vault);
  checkParity(snapshot);
});
test('Umami parity: a newly added vault verb and removed catalog entry fail', () => {
  assert.throws(() => checkParity(vault.replace('## Läsytor', '| new-verb | `POST /new-verb` | `new` | ✓ | |\n\n## Läsytor')), /Unmapped vault verb: new-verb/);
  assert.throws(() => checkParity(vault, VERB_EVENTS.filter(entry => entry.verb !== 'build')), /No event for vault verb: build/);
});

const ID = '12345678-1234-1234-1234-123456789abc';
const W = `/api/v1/worlds/${ID}/`;
const tick = () => new Promise(resolve => setImmediate(resolve));
function harness(t, handler) {
  const saved = {};
  for (const key of ['window', 'document', 'localStorage', 'fetch']) saved[key] = globalThis[key];
  t.after(() => { for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete globalThis[key]; else globalThis[key] = value;
  } });
  const events = [];
  globalThis.window = { umami: { track: (name, props) => events.push({ name, props }) } };
  globalThis.document = { getElementById: () => null };
  globalThis.localStorage = { getItem: () => 'private-token' };
  globalThis.fetch = handler;
  return events;
}

function bodyFor(verb) {
  if (verb === 'staff') return { target_kind: 'building', building_type: 'foundry', good_key: 'bronze' };
  if (verb === 'place') return { target_kind: 'hex', hex_ordinal: 7, good_key: 'grain' };
  if (verb === 'trade-offer') return { trade_offer: { kind: 'sell', offer_good: 'tin' } };
  if (verb === 'redirect') return { target_q: 9, target_r: 4 };
  return {};
}
for (const entry of VERB_EVENTS) {
  test(`Umami success: ${entry.verb} ${entry.method} ${entry.route}`, async t => {
    const response = new Response(JSON.stringify({ kind: entry.verb === 'gift/tribute' ? 'gift' : 'transfer', secret: ID }), { status: 202 });
    const events = harness(t, async () => response);
    const opts = { method: entry.method, body: JSON.stringify(bodyFor(entry.verb)) };
    const res = await fetchAuth(entry.route.replace(/:[a-z]+/g, ID), opts);
    await tick();
    assert.equal(res, response);
    assert.equal(events.length, 1, 'each accepted API request must emit exactly one verb event');
    assert.equal(events[0].name, entry.event);
    assert.ok(!JSON.stringify(events).includes(ID), 'no route or response UUID may leave the client');
    assert.equal((await res.json()).secret, ID, 'the caller must retain its unread response');
  });
}

test('Umami preserves all eight historical verb event names', () => {
  for (const event of ['build_started', 'recruit_started', 'march_sent', 'messenger_sent', 'trade_offer', 'rite_performed', 'settle', 'livestock_slaughtered']) {
    assert.ok(VERB_EVENTS.some(entry => entry.event === event), event);
  }
});
test('Umami counts requests only after OK, including bulk partial success and 204', async t => {
  let finish;
  const events = harness(t, () => new Promise(resolve => { finish = resolve; }));
  const pending = fetchAuth(W + `units/${ID}/recall`, { method: 'POST', body: '{}' });
  assert.deepEqual(events, [], 'a click/in-flight request is not a successful verb');
  finish(new Response(null, { status: 204 }));
  await pending;
  assert.deepEqual(events, [{ name: 'recall_sent', props: undefined }]);
  globalThis.fetch = async () => new Response('{"error":"no route"}', { status: 422 });
  await fetchAuth(W + `units/${ID}/recall`, { method: 'POST', body: '{}' });
  await tick();
  assert.deepEqual(events[1], { name: 'verb_refused', props: { verb: 'recall' } });
});
test('Umami drops IDs, names, passwords, free text, positions and arbitrary enum lookalikes', async t => {
  const events = harness(t, async () => new Response('{}'));
  const body = { building_type: ID, unit_type: 'Private Spear', name: 'private name', message: 'private letter', prayer: ID, intent: ID, good_key: 'secret_good', old_password: 'old-secret', new_password: 'new-secret', target_q: 123, target_r: 456 };
  for (const [route, method] of [['provinces/:id/build', 'POST'], ['provinces/:id/recruit', 'POST'], ['settlements/:id/rite', 'POST'], ['units/:id/march', 'POST'], ['founding/messengers', 'POST']]) {
    await fetchAuth(W + route.replace(':id', ID), { method, body: JSON.stringify(body) });
  }
  await fetchAuth('/api/v1/auth/password', { method: 'POST', body: JSON.stringify(body), sensitive: true });
  assert.ok(events.every(event => event.props === undefined));
  assert.equal(events.length, 6);
});
test('Umami retains only known public category values', async t => {
  const events = harness(t, async () => new Response('{}'));
  for (const [route, body] of [
    ['provinces/:id/build', { building_type: 'farm' }],
    ['provinces/:id/recruit', { unit_type: 'galley', name: 'SECRET' }],
    ['units/:id/march', { intent: 'explore', ticks: 40 }],
    ['settlements/:id/rite', { prayer: 'minoan_harvest_blessing' }],
  ]) await fetchAuth(W + route.replace(':id', ID), { method: 'POST', body: JSON.stringify(body) });
  assert.deepEqual(events.map(event => event.props), [{ building: 'farm' }, { unit: 'galley' }, { intent: 'explore' }, { rite: 'minoan_harvest_blessing' }]);
});
test('Umami refusal reason_code is whitelisted server token, never response prose or IDs', async t => {
  const replies = [
    { error: 'insufficient_goods', missing: [{ good: 'silver', need: 100, have: 3 }], id: ID },
    { error_code: 'insufficient_goods', error: 'private message' },
    { error_code: ID, error: `private city ${ID}` },
    { error: 'private name cannot march' },
    null,
  ];
  const events = harness(t, async () => new Response(JSON.stringify(replies.shift()), { status: 422 }));
  for (let i = 0; i < 5; i++) await fetchAuth(W + `provinces/${ID}/build`, { method: 'POST' });
  await tick();
  assert.deepEqual(events.map(event => event.props), [
    { verb: 'build', reason_code: 'insufficient_goods' }, { verb: 'build', reason_code: 'insufficient_goods' },
    { verb: 'build' }, { verb: 'build' }, { verb: 'build' },
  ]);
  assert.ok(events.every(event => event.name === 'verb_refused'));
});
test('Umami ignores GET, wrong method, unknown routes and gated kingdoms', async t => {
  const events = harness(t, async () => new Response('{}'));
  for (const [url, method] of [[W + `units/${ID}/march`, 'GET'], [W + `units/${ID}/march`, 'PUT'], [W + 'kingdoms', 'POST'], [W + 'unknown', 'POST']]) {
    await fetchAuth(url, { method });
    assert.equal(verbForRequest(url, { method }), null);
  }
  assert.deepEqual(events, []);
});
test('Umami never reports success/refusal for 5xx or network failure', async t => {
  const events = harness(t, async () => new Response('{}', { status: 503 }));
  await fetchAuth(W + `units/${ID}/march`, { method: 'POST' });
  globalThis.fetch = async () => { throw new Error('offline'); };
  await assert.rejects(fetchAuth(W + `units/${ID}/march`, { method: 'POST' }), /offline/);
  assert.ok(events.every(event => event.name === 'fetch_fail'));
});
test('Umami bodyless/non-JSON refusals, transfers and adblockers never break requests', async t => {
  const events = harness(t, async () => new Response('not JSON', { status: 400 }));
  const res = await fetchAuth(W + `units/${ID}/march`, { method: 'POST' });
  await tick();
  assert.deepEqual(events[0], { name: 'verb_refused', props: { verb: 'march' } });
  assert.equal(await res.text(), 'not JSON');
  globalThis.fetch = async () => new Response(null, { status: 204 });
  await fetchAuth(W + `provinces/${ID}/trade`, { method: 'POST' });
  await tick();
  assert.equal(events[1].name, 'transfer_sent');
  for (const tracker of [undefined, { track: () => { throw new Error('blocked'); } }, { track: () => Promise.reject(new Error('blocked async')) }]) {
    globalThis.window.umami = tracker;
    assert.equal((await fetchAuth(W + `units/${ID}/march`, { method: 'POST' })).status, 204);
    await tick();
  }
});
test('Umami shared response boundary owns verb tracking and former bare-fetch paths', () => {
  const api = readFileSync(new URL('api.js', import.meta.url), 'utf8');
  assert.match(api, /trackVerbResponse\(url, opts, res\)/);
  const join = readFileSync(new URL('web/templates/join.html', root), 'utf8');
  assert.match(join, /window\.joinWorld = async function joinWorld/);
  assert.match(join, /await fetchAuth\('\/api\/v1\/worlds\//);
  for (const path of ['ui/drawers/city.js', 'ui/drawers/war.js', 'ui/drawers/diplomacy.js', 'ui/drawers/kult.js', 'ui/marchctx.js', 'render/map.js']) {
    const source = readFileSync(new URL(path, import.meta.url), 'utf8');
    assert.doesNotMatch(source, /\btrack\(/, 'former callers must not double-count accepted requests');
  }
  assert.match(readFileSync(new URL('render/map.js', import.meta.url), 'utf8'), /await fetchAuth\(sendPath,/);
});
