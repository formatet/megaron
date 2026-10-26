import test from 'node:test';
import assert from 'node:assert/strict';
import { sentStatusHTML, orderRunnerHTML } from './runner_status.js';

const past = new Date(Date.now() - 3600e3).toISOString();
const future = new Date(Date.now() + 3600e3).toISOString();

// The server's messenger lifecycle is outbound → delivered → returning →
// arrived. The outbox used to match 'delivering'/'returned' and fell through
// to the raw word for every real row.
test('outbox: an outbound runner on the road shows its arrival, never the raw status', () => {
  const s = sentStatusHTML({ status: 'outbound', arrives_at: future });
  assert.match(s, /en route · arrives/);
  assert.doesNotMatch(s, /outbound/);
});

test('outbox: a runner waiting in port names the port, not a past arrival', () => {
  // arrives_at is the moment it reached the port — already past.
  const s = sentStatusHTML({ status: 'outbound', arrives_at: past, passage_status: 'awaiting_passage', passage_port: 'Amnisos' });
  assert.match(s, /waiting in Amnisos for a ship/);
  assert.doesNotMatch(s, /arrived|arrives/);
});

test('outbox: a runner aboard names its ship', () => {
  const s = sentStatusHTML({ status: 'outbound', arrives_at: future, passage_status: 'aboard', carrier_name: 'Swift Dolphin' });
  assert.match(s, /aboard Swift Dolphin/);
});

test('outbox: delivered, returning, arrived each read as what they are', () => {
  assert.match(sentStatusHTML({ status: 'delivered', arrives_at: past }), /delivered · waiting on a reply/);
  assert.match(sentStatusHTML({ status: 'returning', arrives_at: future }), /coming home · back/);
  assert.match(sentStatusHTML({ status: 'arrived', arrives_at: past }), /↩ home/);
});

test('outbox: a returning runner waiting for a ship home says so', () => {
  const s = sentStatusHTML({ status: 'returning', arrives_at: past, passage_status: 'awaiting_passage', passage_port: 'Pylos' });
  assert.match(s, /coming home · waiting in Pylos for a ship/);
});

test('outbox: a sealed runner (ship lost) is not shown as arriving', () => {
  const s = sentStatusHTML({ status: 'outbound', arrives_at: past, passage_status: 'returning_sealed', passage_port: 'Amnisos' });
  assert.match(s, /ship was lost/);
  assert.doesNotMatch(s, /arrive/);
});

test('outbox: a port name is escaped', () => {
  assert.doesNotMatch(sentStatusHTML({ status: 'outbound', passage_status: 'awaiting_passage', passage_port: '<b>' }), /<b>/);
});

// The bug: arrives_at (port arrival) passed → "carrying the order…" while
// the runner stood on the quay.
test('order card: a runner waiting for a ship never claims to be carrying the order', () => {
  const s = orderRunnerHTML({ arrives_at: past, passage_status: 'awaiting_passage', passage_port: 'Amnisos' }, Date.now());
  assert.doesNotMatch(s, /carrying the order/);
  assert.match(s, /waiting in Amnisos for a ship — the order has not reached the unit/);
});

test('order card: the ordinary land runner keeps its two states', () => {
  assert.match(orderRunnerHTML({ arrives_at: future }, Date.now()), /Messenger en route — order arrives/);
  assert.match(orderRunnerHTML({ arrives_at: past }, Date.now()), /Messenger carrying the order…/);
});

test('T2: rescue uncertainty says no word without an ETA or carrier hint', () => {
  const s = sentStatusHTML({ status: 'outbound', passage_status: 'unknown', arrives_at: future, carrier_name: 'SECRET SHIP', passage_port: 'SECRET PORT' });
  assert.match(s, /no word/);
  assert.doesNotMatch(s, /SECRET|arrives|aboard|sealed/);
});

test('order pursuit: next meeting time never promises delivery to a moving unit', () => {
  const s = orderRunnerHTML({ following_unit: true, pursuit_legs: 0, arrives_at: future }, Date.now());
  assert.match(s, /Runner following the unit/);
  assert.match(s, /next meeting/);
  assert.doesNotMatch(s, /order arrives|reaches the unit|carrying the order/);
});

test('order pursuit: rerouted Runner keeps following after an old meeting time passes', () => {
  const s = orderRunnerHTML({ following_unit: true, pursuit_legs: 2, arrives_at: past }, Date.now());
  assert.match(s, /Runner following the unit/);
  assert.match(s, /new route/);
  assert.match(s, /checking the next meeting point/);
  assert.doesNotMatch(s, /order arrives|carrying the order|delivered/);
});

test('order pursuit: waiting for passage keeps the physical wait and hides a false meeting ETA', () => {
  const s = orderRunnerHTML({ following_unit: true, pursuit_legs: 1, arrives_at: future, passage_status: 'awaiting_passage', passage_port: '<Pylos>' }, Date.now());
  assert.match(s, /Runner following the unit/);
  assert.match(s, /waiting in &lt;Pylos&gt; for a ship/);
  assert.doesNotMatch(s, /next meeting|order arrives|<Pylos>/);
});

test('order pursuit: absent meeting time remains explicitly unknown', () => {
  const s = orderRunnerHTML({ following_unit: true, pursuit_legs: 1, arrives_at: '' }, Date.now());
  assert.match(s, /meeting time unavailable/);
  assert.doesNotMatch(s, /Invalid Date|NaN|order arrives/);
});

test('order pursuit: temporary wait has no meeting or delivery ETA',()=>{
 const s=orderRunnerHTML({following_unit:true,following_wait:true,pursuit_legs:2,arrives_at:future},Date.now());
 assert.match(s,/Runner waiting to continue following the unit/);
 assert.doesNotMatch(s,/next meeting|order arrives|today|tomorrow|checking/);
});
