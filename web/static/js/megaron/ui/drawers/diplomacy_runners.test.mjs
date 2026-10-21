import test from 'node:test';
import assert from 'node:assert/strict';
import { State } from '../../state.js';
import { runnersTabHTML, loadDipRunners } from './diplomacy_runners.js';
import { loadDiplomacyDrawer } from './diplomacy.js';

const iso = h => new Date(Date.now() + h * 3600000).toISOString();
const DATA = {
  own: [
    { id: 'w1', group: 'waiting', kind: 'letter', to_city: 'Ugarit', to_wanax: 'Kassandra', port_name: 'Byblos', waiting_since: iso(-3),
      explanation: 'Your runner is waiting in Byblos: no ship has sailed from there toward Ugarit.', can_call_back: true, can_arrange_passage: true,
      eligible_ships: [{ id: 's1', name: 'Dawn Gull', settlement_name: 'Byblos' }] },
    { id: 'w2', group: 'waiting', kind: 'letter', to_city: 'Ugarit', to_wanax: 'Kassandra', port_name: 'Tyre', waiting_since: iso(-1),
      explanation: 'Waiting in Tyre.', can_call_back: false, can_arrange_passage: true, eligible_ships: [] },
    { id: 'r1', group: 'on_the_way', kind: 'order', order_summary: 'Third Spears: Set stance: hold', arrives_at: iso(2) },
    { id: 'a1', group: 'aboard', kind: 'offer', to_city: 'Ugarit', to_wanax: 'Kassandra', ship_name: 'Sea Swallow', ship_wanax: 'Kassandra', boarded_at: iso(-2), lands_at: iso(5), arrives_at: iso(6) },
    { id: 'h1', group: 'homebound', kind: 'letter', to_city: 'Byblos', arrives_at: iso(1) },
  ],
  foreign: [
    { group: 'in_your_city', sender_wanax: 'Kassandra', where: 'Byblos', waiting_for: 'a_ship', since: iso(-4) },
    { group: 'aboard_your_ship', sender_wanax: 'Dunios', ship_name: 'Sea Swallow', heading_to: 'Ugarit', lands_at: iso(5) },
  ],
};

test('Runners tab: every group, server explanation, buttons only where the server allows', () => {
  const html = runnersTabHTML(DATA);
  for (const want of ['Waiting for a ship', 'On the way', 'Aboard a ship', 'On the way home', 'Others’ runners in your cities', 'Others’ runners aboard your ships',
    'no ship has sailed from there toward Ugarit', 'Third Spears: Set stance: hold', 'To Wanax Kassandra (Ugarit)', 'aboard Wanax Kassandra’s Sea Swallow',
    'Wanax Kassandra’s runner in Byblos', 'Wanax Dunios’ runner aboard Sea Swallow', 'bound for Ugarit', 'Home to Byblos']) {
    assert.ok(html.includes(want), `missing ${want}`);
  }
  assert.equal((html.match(/Arrange passage/g) || []).length, 1, 'passage button only for the runner with an eligible ship');
  assert.equal((html.match(/Call back/g) || []).length, 1, 'call-back only where can_call_back');
  assert.match(html, /dipArrangePassage\('w1','dip-runner-ship-w1',this,'dip-runner-block-w1'\)/);
  assert.match(html, /dipCallBack\('w1',this\)/);
  assert.doesNotMatch(html, /[0-9a-f]{8}-[0-9a-f]{4}-/, 'no uuid in player text');
  assert.doesNotMatch(html, /\bday \d/i);
});

test('Runners tab: names are escaped; empty state', () => {
  const html = runnersTabHTML({ own: [], foreign: [{ group: 'in_your_city', sender_wanax: '<b>X</b>', where: 'A&B', waiting_for: 'your_answer' }] });
  assert.ok(!html.includes('<b>X'));
  assert.ok(html.includes('&lt;b&gt;X'));
  assert.match(runnersTabHTML({ own: [], foreign: [] }), /No runners/);
});

test('the real drawer has a Runners tab that fetches /runners', async () => {
  const original = { fetch: globalThis.fetch, document: globalThis.document, localStorage: globalThis.localStorage };
  const tabs = [];
  const body = { innerHTML: '', querySelectorAll: sel => (sel === '.dtab' ? tabs : []) };
  const threads = { innerHTML: '', querySelectorAll: () => [], insertAdjacentHTML() {} };
  const runners = { innerHTML: '' };
  globalThis.document = { getElementById: id => ({ 'diplomacy-body': body, 'dtab-threads': threads, 'dtab-runners': runners })[id] || null };
  globalThis.localStorage = { getItem: () => null };
  State.WORLD_ID = 'w'; State.provinceData = []; State.MY_SETTLEMENT_ID = null;
  const calls = [];
  globalThis.fetch = async url => { calls.push(url); return new Response(JSON.stringify(url.endsWith('/runners') ? DATA : []), { status: 200 }); };
  try {
    await loadDiplomacyDrawer();
    assert.match(body.innerHTML, /data-tab="runners">Runners</);
    await loadDipRunners();
    assert.ok(calls.some(u => u.endsWith('/worlds/w/runners')));
    assert.match(runners.innerHTML, /Waiting for a ship/);
  } finally { Object.assign(globalThis, original); State.WORLD_ID = null; }
});
