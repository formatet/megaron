import test from 'node:test';
import assert from 'node:assert/strict';
import { openAccountWindow, closeAccountWindow } from './account_window.js';
import { diagnosticsSnapshot } from './diagnostics.js';
import { openDispatchWindow } from './dispatch_window.js';

// Small DOM harness executes the real controller and fetchAuth, without a browser
// dependency. The acceptance rig separately exercises actual HTML and services.
function harness(t, handler) {
  const nodes = new Map();
  class Node {
    constructor(id) { this.id = id; this.hidden = true; this.textContent = ''; this.events = {}; this.children = []; this.classes = new Set(); this.classList = { add: x => this.classes.add(x), remove: x => this.classes.delete(x), contains: x => this.classes.has(x) }; }
    set innerHTML(value) {
      this.html = value;
      this.children.forEach(id => nodes.delete(id)); this.children = [];
      for (const tag of value.matchAll(/<[^>]+\bid="([^"]+)"[^>]*>/g)) {
        const id = tag[1]; const child = new Node(id); child.hidden = /\shidden(?:\s|>)/.test(tag[0]); nodes.set(id, child); this.children.push(id);
      }
    }
    get innerHTML() { return this.html || ''; }
    addEventListener(type, fn) { this.events[type] = fn; }
    click() { return this.events.click?.({ preventDefault() {} }); }
  }
  nodes.set('account-window-overlay', new Node('account-window-overlay'));
  nodes.set('account-window-body', new Node('account-window-body'));
  const changes = [], requests = [];
  const previous = Object.fromEntries(['document', 'localStorage', 'window', 'fetch'].map(k => [k, globalThis[k]]));
  globalThis.document = { getElementById: id => nodes.get(id) || null };
  globalThis.localStorage = { getItem: () => 'game-token', setItem: (...args) => changes.push(['set', ...args]), removeItem: key => changes.push(['remove', key]) };
  globalThis.window = { location: {} };
  globalThis.fetch = async (url, options) => { requests.push({ url, options }); return handler(url, options); };
  t.after(() => { closeAccountWindow(); Object.assign(globalThis, previous); });
  return { nodes, requests, changes };
}
const response = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const ready = { enabled: true, state: 'ready', user_id: '@agamemnon:agora.test', homeserver: 'agora.test' };
const settle = () => new Promise(resolve => setImmediate(resolve));

for (const state of ['disabled', 'pending', 'provisioning']) {
  test(`chat ${state}: no password action and no password POST`, async t => {
    const h = harness(t, () => response(state === 'disabled' ? { enabled: false } : { enabled: true, state }));
    openAccountWindow(); await settle();
    assert.equal(h.nodes.has('acc-chat-get'), false);
    assert.equal(h.nodes.get('acc-chat').hidden, state === 'disabled');
    assert.equal(h.requests.length, 1);
    assert.equal(h.requests[0].url, '/api/v1/agora');
    assert.equal(h.requests[0].options.headers.Authorization, 'Bearer game-token');
    assert.deepEqual(h.changes, []);
  });
}

test('ready: only an explicit click requests a password; close erases it', async t => {
  const h = harness(t, url => response(url.endsWith('/password') ? { password: 'ephemeral-one', user_id: ready.user_id } : ready));
  openAccountWindow(); await settle();
  assert.equal(h.requests.length, 1);
  await h.nodes.get('acc-chat-get').click();
  assert.equal(h.nodes.get('acc-chat-password').textContent, 'ephemeral-one');
  assert.equal(h.requests[1].options.method, 'POST');
  assert.equal(h.requests[1].options.cache, 'no-store');
  assert.equal('sensitive' in h.requests[1].options, false, 'internal safety option is not sent to fetch');
  closeAccountWindow();
  assert.equal(h.nodes.get('acc-chat-password').textContent, '');
  assert.equal(h.nodes.get('acc-chat-password').hidden, true);
  assert.deepEqual(h.changes, []);
});

test('close/reopen during password request discards the late secret', async t => {
  let resolve;
  const h = harness(t, url => url.endsWith('/password') ? new Promise(r => resolve = r) : response(ready));
  openAccountWindow(); await settle();
  const request = h.nodes.get('acc-chat-get').click();
  closeAccountWindow(); openAccountWindow(); await settle();
  resolve(response({ password: 'late-secret' })); await request;
  assert.equal(h.nodes.get('acc-chat-password').textContent, '');
  assert.equal(h.nodes.get('acc-chat-password').hidden, true);
});

test('another request clears the first secret immediately; errors bypass diagnostics', async t => {
  let calls = 0, resolve;
  const h = harness(t, url => {
    if (!url.endsWith('/password')) return response(ready);
    return ++calls === 1 ? response({ password: 'first-secret' }) : new Promise(r => resolve = r);
  });
  openAccountWindow(); await settle(); await h.nodes.get('acc-chat-get').click();
  const request = h.nodes.get('acc-chat-get').click();
  assert.equal(h.nodes.get('acc-chat-password').textContent, '');
  resolve(response({ error: 'upstream echoed private-secret' }, 503)); await request; await settle();
  assert.equal(h.nodes.get('acc-chat-password').hidden, true);
  assert.ok(!JSON.stringify(diagnosticsSnapshot()).includes('private-secret'));
  assert.ok(!h.nodes.get('acc-chat-status').textContent.includes('private-secret'));
});

test('sign out clears the displayed secret before navigation', async t => {
  const h = harness(t, url => response(url.endsWith('/password') ? { password: 'signout-secret' } : ready));
  openAccountWindow(); await settle(); await h.nodes.get('acc-chat-get').click();
  h.nodes.get('acc-signout-btn').click();
  assert.equal(h.nodes.get('acc-chat-password').textContent, '');
  assert.equal(window.location.href, '/logout');
  assert.deepEqual(h.changes, [['remove', 'poleia_token'], ['remove', 'poleia_refresh']]);
});

test('agora_ready dispatch opens Account rather than toggling an already open window', async t => {
  const h = harness(t, () => response(ready));
  const Node = h.nodes.get('account-window-overlay').constructor;
  h.nodes.set('dispatch-window-overlay', new Node('dispatch-window-overlay'));
  h.nodes.set('dw-body', new Node('dw-body'));
  openAccountWindow(); await settle();
  openDispatchWindow('agora_ready', { user_id: ready.user_id }, 'now');
  h.nodes.get('dw-account-btn').click(); await settle();
  assert.equal(h.nodes.get('account-window-overlay').classList.contains('open'), true);
  assert.equal(h.nodes.get('dispatch-window-overlay').classList.contains('open'), false);
  assert.ok(h.nodes.get('acc-chat-get'));
  assert.ok(h.requests.every(r => !r.url.endsWith('/password')));
});

test('ReadyRenderedLinkUsesConfiguredHomeserver', async t => {
  const homeserver = 'http://127.0.0.1:18100';
  const h = harness(t, () => response({ ...ready, homeserver }));
  openAccountWindow(); await settle();
  assert.ok(h.nodes.get('acc-chat').innerHTML.includes(`href="${homeserver}"`));
  assert.ok(!h.nodes.get('acc-chat').innerHTML.includes('https://agora.formatet.se'));
  assert.ok(h.requests.every(r => !r.url.endsWith('/password')));
});
