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
  assert.ok(text.includes(loss.envelope.sent_at));
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
  assert.match(text, /home/);
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
    assert.ok(text.includes(body.envelope.sent_at));
    for (const key of ['trade_offer', 'order_payload']) {
      if (body.envelope[key]) assert.ok(text.includes(JSON.stringify(body.envelope[key], null, 2)));
    }
  });
}
