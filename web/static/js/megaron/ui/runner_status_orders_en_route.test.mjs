import test from 'node:test';
import assert from 'node:assert/strict';
import { ordersEnRouteHTML } from './runner_status.js';

const future = new Date(Date.now() + 3600e3).toISOString();

test('orders_en_route: a march names verb and target; empty renders nothing', () => {
  const s = ordersEnRouteHTML([{ verb: 'march', target_q: 47, target_r: 23, arrives_at: future }]);
  assert.match(s, /Order on its way: march to \(47,23\) — the runner reaches it/);
  assert.match(s, /class="order-en-route"/);
  assert.equal(ordersEnRouteHTML([]), '');
  assert.equal(ordersEnRouteHTML(undefined), '');
});

test('orders_en_route: recall and stance read in plain words', () => {
  assert.match(ordersEnRouteHTML([{ verb: 'recall', arrives_at: future }]), /recall home/);
  assert.match(ordersEnRouteHTML([{ verb: 'stance_pursuit', arrives_at: future }]), /stance change/);
});
