import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { messengerEnvelopeText, notifText } from './format.js';

// Exported from the actual database notification rows by carrier_fate_test.go.
const loss = JSON.parse(readFileSync(new URL('./testdata/messenger_lost_at_sea.json', import.meta.url)));
const rescue = JSON.parse(readFileSync(new URL('./testdata/messenger_rescued_at_sea.json', import.meta.url)));

test('T2: loss dispatch renders the WHOLE frozen sealed letter and endpoints', () => {
  const text = messengerEnvelopeText(loss);
  assert.ok(text.includes(loss.envelope.message_text));
  assert.ok(text.includes(loss.envelope.origin.name));
  assert.ok(text.includes(loss.envelope.destination.name));
  assert.ok(text.includes(`Sent on day ${loss.envelope.sent_tick}`));
  assert.ok(!text.includes(loss.envelope.sent_at));
  assert.match(notifText('MessengerLostAtSea', loss), /lost at sea in a storm/);
  assert.doesNotMatch(text, /changed AFTER loss/);
});

test('T2: full trade and unit order terms survive rendering, including long contents', () => {
  const letter = '<script>untrusted</script>' + 'long letter '.repeat(200);
  const body = { envelope: { ...loss.envelope, message_text: letter, trade_offer: {kind:'sell',offer_good:'bronze',offer_qty:31,want_silver:145.7}, order_payload:{verb:'march',unit_id:'physical-unit',q:9,r:4,standing_orders:{hold_to_last_man:true}} }};
  const text = messengerEnvelopeText(body);
  assert.ok(text.includes(letter));
  for (const value of ['bronze','145.7','march','physical-unit','hold_to_last_man']) assert.ok(text.includes(value));
});

test('T2: rescue report names frozen ship and actual port after home return', () => {
  const text = notifText('MessengerRescuedAtSea', rescue);
  assert.ok(text.includes(`Home on day ${rescue.home_tick}.`));
  assert.match(text, /Sacred Dolphin/);
  const port = rescue.journey.find(j => j.port).port;
  assert.ok(text.includes(port));
  assert.doesNotMatch(text, /renamed AFTER/);
});

for (const file of ['messenger_lost_trade.json', 'messenger_lost_order.json']) {
  test(`T2: actual archived ${file} preserves every sealed parameter`, () => {
    const body = JSON.parse(readFileSync(new URL(`./testdata/${file}`, import.meta.url)));
    const text = messengerEnvelopeText(body);
    assert.ok(text.includes(body.envelope.message_text));
    assert.ok(text.includes(`Sent on day ${body.envelope.sent_tick}`));
    for (const key of ['trade_offer', 'order_payload']) {
      if (body.envelope[key]) assert.ok(text.includes(JSON.stringify(body.envelope[key], null, 2)));
    }
  });
}

test('T2: missing legacy departure day is explicit, never UTC or a guessed day', () => {
  const body = { envelope: { ...loss.envelope, sent_tick: null } };
  assert.match(messengerEnvelopeText(body), /Sent on an unknown day/);
  assert.ok(!messengerEnvelopeText(body).includes(body.envelope.sent_at));
  assert.match(messengerEnvelopeText({ envelope: { sent_tick: 0 } }), /Sent on day 0/);
});

test('T2: frozen public persons come first in sealed envelope and headline', () => {
  const expected = 'you, at Mycenae, to Wanax Oledoledoff at Tiryns';
  assert.ok(messengerEnvelopeText(loss).includes('From ' + expected));
  assert.equal(notifText('MessengerLostAtSea', loss), 'Your runner to Wanax Oledoledoff at Tiryns was lost at sea in a storm.');
  for (const text of [messengerEnvelopeText(loss), notifText('MessengerLostAtSea', loss), notifText('MessengerRescuedAtSea', rescue)]) {
    assert.doesNotMatch(text, /Passage-|private-login-|Changed After Loss|Renamed After Loss|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}/i);
  }
});
test('T2: order recipient is your named unit at the original coordinates', () => {
  const body = JSON.parse(readFileSync(new URL('./testdata/messenger_lost_order.json', import.meta.url)));
  assert.ok(messengerEnvelopeText(body).includes('From you, at Mycenae, to your Bronze Guard at (9, 4)'));
  assert.equal(notifText('MessengerLostAtSea', body), 'Your runner to your Bronze Guard at (9, 4) was lost at sea in a storm.');
  for (const text of [messengerEnvelopeText(body), notifText('MessengerLostAtSea', body)]) {
    assert.doesNotMatch(text, /Changed Guard|Wanax Atreus|to Wanax/);
  }
});
test('T2: missing public recipient stays unknown without guessing a login or UUID', () => {
  const body = {envelope:{origin:{name:'Mycenae'},destination:{name:'Tiryns',username:'private-login',id:'00000000-0000-0000-0000-000000000009'}}};
  const text = messengerEnvelopeText(body);
  assert.match(text, /From you, at Mycenae, to an unknown Wanax at Tiryns/);
  assert.doesNotMatch(text, /private-login|00000000/);
});

for (const [label, journey, ending] of [
  ['one landing', rescue.journey.slice(0, 2), ' and put ashore at Tiryns.'],
  ['home landing', rescue.journey, ' and put ashore at Tiryns, then went ashore at Mycenae.'],
]) {
  test(`T2: rescue sentence from frozen fields with ${label}`, () => {
    const body = { ...rescue, journey };
    assert.equal(notifText('MessengerRescuedAtSea', body), `Home on day ${rescue.home_tick}. Your runner was rescued at sea by the Sacred Dolphin${ending}`);
  });
}
test('T2: rescue sentence escapes frozen names for HTML display', () => {
  const text = notifText('MessengerRescuedAtSea', {home_tick:0, journey:[{ship:'<Dolphin>'},{port:'<Tiryns>'}]});
  assert.equal(text, 'Home on day 0. Your runner was rescued at sea by the &lt;Dolphin&gt; and put ashore at &lt;Tiryns&gt;.');
});

test('T2: multiple rescue ships and ports stay ordered without inventing a home port', () => {
  const text = notifText('MessengerRescuedAtSea', {home_tick:509, journey:[{ship:'Sacred Dolphin'},{port:'Tiryns'},{ship:'Blue Gull'},{port:'Knossos'},{port:'Mycenae'}]});
  assert.equal(text, 'Home on day 509. Your runner was rescued at sea by the Sacred Dolphin and put ashore at Tiryns, then was rescued at sea by the Blue Gull and put ashore at Knossos, then went ashore at Mycenae.');
});
