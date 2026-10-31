import { State as timeTestState } from './../state.js';
import { fmtTickTime as testTickTime } from './fmt_clock.js';
timeTestState.CURRENT_TICK=0; timeTestState.TICK_SECONDS=60; timeTestState.TICK_ANCHOR_MS=Date.now();
import test from 'node:test';
import { State } from '../state.js';
import assert from 'node:assert/strict';
import { openAccountWindow, closeAccountWindow } from './account_window.js';
import { diagnosticsSnapshot } from './diagnostics.js';
import { openDispatchWindow } from './dispatch_window.js';

// Small DOM harness executes the real controller and fetchAuth, without a browser
// dependency. The acceptance rig separately exercises actual HTML and services.
function harness(t, handler) {
  const previousWorld = State.WORLD_ID;
  State.WORLD_ID = null;
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
  t.after(() => { State.WORLD_ID = previousWorld; closeAccountWindow(); Object.assign(globalThis, previous); });
  return { nodes, requests, changes };
}
const response = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = () => new Promise(resolve => setImmediate(resolve));

test('community section links Discord and requests no chat account', async t => {
  const h = harness(t, () => response({ enabled: false }));
  openAccountWindow(); await settle();
  const html = h.nodes.get('account-window-body').innerHTML;
  assert.match(html, /href="https:\/\/discord\.gg\/NxtftQUMYW"/);
  assert.doesNotMatch(html, /chat password|Homeserver|acc-chat-get/i);
  assert.ok(h.requests.every(r => !r.url.includes('/agora')));
});

test('sign out clears tokens before navigation', async t => {
  const h = harness(t, () => response({ enabled: false }));
  openAccountWindow(); await settle();
  h.nodes.get('acc-signout-btn').click();
  assert.equal(window.location.href, '/logout');
  assert.deepEqual(h.changes, [['remove', 'poleia_token'], ['remove', 'poleia_refresh']]);
});

test('agora_ready dispatch opens Account rather than toggling an already open window', async t => {
  const h = harness(t, () => response({ enabled: false }));
  const Node = h.nodes.get('account-window-overlay').constructor;
  h.nodes.set('dispatch-window-overlay', new Node('dispatch-window-overlay'));
  h.nodes.set('dw-body', new Node('dw-body'));
  openAccountWindow(); await settle();
  await openDispatchWindow('agora_ready', { user_id: '@agamemnon:agora.test' }, 'now');
  assert.equal(h.nodes.get('dw-action-btn').hidden, false);
  h.nodes.get('dw-action-btn').click(); await settle();
  assert.equal(h.nodes.get('account-window-overlay').classList.contains('open'), true);
  assert.equal(h.nodes.get('dispatch-window-overlay').classList.contains('open'), false);
});

const profile = { wanax_name: 'Nestor', founded_tick: 0, cities: 2, population: 320, kharis: 4.5 };
test('account loads five own-world profile rows, including a real game day zero', async t => {
  const h = harness(t, url => response(url.endsWith('/me') ? profile : { enabled: false }));
  State.WORLD_ID = 'my-world'; openAccountWindow(); await settle();
  const html = h.nodes.get('acc-profile').innerHTML;
  assert.equal((html.match(/class="stat-row"/g) || []).length, 5);
  assert.match(html, new RegExp('Since '+testTickTime(0))); assert.match(html, /Nestor/);
  assert.equal(h.requests.find(r => r.url.endsWith('/me')).url, '/api/v1/worlds/my-world/me');
});
for (const change of ['close', 'world']) {
  test(`late profile ignored after ${change}`, async t => {
    let resolve;
    const h = harness(t, url => url.endsWith('/me') ? new Promise(r => resolve = r) : response({ enabled: false }));
    State.WORLD_ID = 'old-world'; openAccountWindow(); await settle();
    if (change === 'close') closeAccountWindow(); else State.WORLD_ID = 'new-world';
    resolve(response(profile)); await settle();
    assert.ok(!h.nodes.get('acc-profile').innerHTML.includes('Nestor'));
  });
}
test('profile error does not invent zero-valued statistics', async t => {
  const h = harness(t, url => response(url.endsWith('/me') ? { error: 'Not a member of this world' } : { enabled: false }, url.endsWith('/me') ? 404 : 200));
  State.WORLD_ID = 'world'; openAccountWindow(); await settle();
  assert.match(h.nodes.get('acc-profile').textContent, /Not a member/);
  assert.equal(h.nodes.get('acc-profile').innerHTML, '');
});

