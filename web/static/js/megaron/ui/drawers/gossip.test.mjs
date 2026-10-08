import test from 'node:test';
import assert from 'node:assert/strict';

// gossip.js's import graph (state.js, api.js, ui/format.js) is side-effect-free
// at module scope, so a plain static import is safe. Since slice O the only live
// consumer is notif.js, which renders rumours as marked rows of the notification
// list via renderRumourRowHTML.
import { hopsLabel, renderRumourRowHTML } from './gossip.js';

const GOSSIP_ROW = {
  id: 'g1',
  source_region: 'Argolid',
  category: 'harvest',
  text: 'Mycenae\'s granaries overflow this season.',
  generated_at: '2026-08-16T10:00:00.000Z',
  importance: 'minor',
  hops: 2,
};

test('hopsLabel: 0 or missing hops → no qualifier (matches keryx cmd_gossip.go hopLabel)', () => {
  assert.equal(hopsLabel(0), '');
  assert.equal(hopsLabel(undefined), '');
  assert.equal(hopsLabel(null), '');
});

test('hopsLabel: 1 hop is singular, N hops is plural', () => {
  assert.equal(hopsLabel(1), '1 hop away');
  assert.equal(hopsLabel(2), '2 hops away');
  assert.equal(hopsLabel(5), '5 hops away');
});

test('renderRumourRowHTML: a row renders region, category, hops and text, marked as Rumour', () => {
  const html = renderRumourRowHTML(GOSSIP_ROW);
  assert.match(html, /Argolid/);
  assert.match(html, /harvest/);
  assert.match(html, /2 hops away/);
  assert.match(html, /Mycenae/);
  assert.match(html, /Rumour/);
  assert.doesNotMatch(html, /gossip-major/, 'importance=minor must not get the major-highlight class');
});

test('renderRumourRowHTML: importance=major gets the highlight class', () => {
  const html = renderRumourRowHTML({ ...GOSSIP_ROW, importance: 'major' });
  assert.match(html, /gossip-major/);
});

test('renderRumourRowHTML: escapes rumour text so a hostile string cannot inject markup', () => {
  const html = renderRumourRowHTML({ ...GOSSIP_ROW, text: '<img src=x onerror=alert(1)>' });
  assert.doesNotMatch(html, /<img/);
});

test('renderRumourRowHTML: a 0-hop rumour carries no distance qualifier', () => {
  assert.doesNotMatch(renderRumourRowHTML({ ...GOSSIP_ROW, hops: 0 }), /hops? away/);
});
