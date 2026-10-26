import test from 'node:test';
import assert from 'node:assert/strict';

// dispatch_window.js has NO top-level DOM/window touches (its whole point —
// see the file's own header comment on why centreOn is reached via
// window.centreOn instead of a direct ui/search.js import), so this file
// needs no globalThis stubbing before import, unlike its DOM-heavy siblings.
import { resolveDestination, letterPersonFor, passageStalledBlockHTML, occupationChoicesFor, buildOccupationOrderBody, openingDispatchHTML } from './dispatch_window.js';
import { State } from '../state.js';

test('AK1: a direct q/r on the payload wins outright, no lookup needed', () => {
  State.provinceData = [];
  assert.deepEqual(resolveDestination('UnitArrived', { q: 5, r: 9 }), { q: 5, r: 9 });
});

test('AK2: settlement_id resolves via State.provinceData', () => {
  State.provinceData = [{ id: 'sid-1', q: 3, r: 4 }];
  assert.deepEqual(resolveDestination('BuildComplete', { settlement_id: 'sid-1' }), { q: 3, r: 4 });
});

test('AK3: dest_id/destination_id (transport/trade kinds) resolve the same way settlement_id does', () => {
  State.provinceData = [{ id: 'sid-2', q: 7, r: 1 }];
  assert.deepEqual(resolveDestination('TradeDelivery', { dest_id: 'sid-2' }), { q: 7, r: 1 });
  assert.deepEqual(resolveDestination('TradeReturn', { destination_id: 'sid-2' }), { q: 7, r: 1 });
});

test('AK4: OfferAccepted/Declined/Expired go to the COUNTERPARTY city, not the recipient\'s own (Timothy 2026-09-04, megaron_plan_dispatches.md §3)', () => {
  State.provinceData = [
    { id: 'own-city', q: 1, r: 1 },
    { id: 'their-city', q: 20, r: 20 },
  ];
  const payload = { settlement_id: 'own-city', counterparty_id: 'their-city' };
  assert.deepEqual(resolveDestination('OfferAccepted', payload), { q: 20, r: 20 });
  assert.deepEqual(resolveDestination('OfferDeclined', payload), { q: 20, r: 20 });
  assert.deepEqual(resolveDestination('OfferExpired', payload), { q: 20, r: 20 });
});

test('AK5: OfferAccepted falls back to settlement_id if counterparty_id is somehow absent', () => {
  State.provinceData = [{ id: 'own-city', q: 1, r: 1 }];
  assert.deepEqual(resolveDestination('OfferAccepted', { settlement_id: 'own-city' }), { q: 1, r: 1 });
});

test('AK6: a non-offer kind never picks up counterparty_id even if present', () => {
  State.provinceData = [
    { id: 'own', q: 2, r: 2 },
    { id: 'other', q: 9, r: 9 },
  ];
  assert.deepEqual(
    resolveDestination('BuildComplete', { settlement_id: 'own', counterparty_id: 'other' }),
    { q: 2, r: 2 },
  );
});

test('AK7: an unresolvable settlement_id falls through to a unit_id lookup', () => {
  State.provinceData = [];
  State.unitsData = [{ id: 'u1', q: 11, r: 12 }];
  assert.deepEqual(resolveDestination('OrderFailed', { settlement_id: 'ghost', unit_id: 'u1' }), { q: 11, r: 12 });
});

test('AK8: a foreign unit (not own) is still found via State.foreignUnitData', () => {
  State.provinceData = [];
  State.unitsData = [];
  State.foreignUnitData = [{ id: 'f1', q: 30, r: 31 }];
  assert.deepEqual(resolveDestination('SentryAlerted', { ship_id: 'f1' }), { q: 30, r: 31 });
});

test('AK9: no destination anywhere returns null (the disabled-button case)', () => {
  State.provinceData = [];
  State.unitsData = [];
  State.foreignUnitData = [];
  assert.equal(resolveDestination('WorldAnnouncement', {}), null);
  assert.equal(resolveDestination('WorldAnnouncement', null), null);
});

// ── occupationChoicesFor (S3/S6, megaron_plan_erovring.md) ──────────────────

test('OC1: CityOccupied as attacker offers sack/burn from the payload\'s own choices, never annex (too early)', () => {
  assert.deepEqual(
    occupationChoicesFor('CityOccupied', { role: 'attacker', choices: ['occupy', 'sack', 'burn'] }),
    ['sack', 'burn'],
  );
});

test('OC2: CityOccupied as defender offers nothing — it is not this Wanax\'s choice to make', () => {
  assert.deepEqual(occupationChoicesFor('CityOccupied', { role: 'defender', choices: ['occupy', 'sack', 'burn'] }), []);
});

test('OC3: CityAnnexReady offers all three — sack/burn remain available once annex unlocks', () => {
  assert.deepEqual(occupationChoicesFor('CityAnnexReady', { settlement_id: 'x' }), ['sack', 'burn', 'annex']);
});

test('OC4: an unrelated dispatch kind offers nothing', () => {
  assert.deepEqual(occupationChoicesFor('BuildComplete', { settlement_id: 'x' }), []);
});

test('OC5: a missing/malformed choices list on CityOccupied degrades to no buttons rather than throwing', () => {
  assert.deepEqual(occupationChoicesFor('CityOccupied', { role: 'attacker' }), []);
  assert.deepEqual(occupationChoicesFor('CityOccupied', {}), []);
});

test('OC6: buildOccupationOrderBody sends exactly {action} — sack loots everything by default, no goods picker', () => {
  assert.deepEqual(buildOccupationOrderBody('sack'), { action: 'sack' });
  assert.deepEqual(buildOccupationOrderBody('burn'), { action: 'burn' });
  assert.deepEqual(buildOccupationOrderBody('annex'), { action: 'annex' });
});

// ── PassageStalled (megaron_plan_ordna_passage.md, slice 3b-4 R5) ──────────
test('PS1: PassageStalled resolves "take me there" via the port q/r the server sends', () => {
  State.provinceData = [];
  assert.deepEqual(
    resolveDestination('PassageStalled', { q: 4, r: 2, port_q: 4, port_r: 2, messenger_id: 'm1' }),
    { q: 4, r: 2 },
  );
});


test('opening dispatch preserves all archived paragraphs and escapes text', () => {
  const paragraphs = ['Time: 7 minutes.', 'Plains: 3 game days.', 'Host: 987 grain.', 'Collapse below 321.', 'The gods: standing fades from the start.', 'Orders travel.', 'Bronze needs others.', '<script>not markup</script> & Report'];
  const html = openingDispatchHTML({ paragraphs, returning: true, host_id: 'hidden-host-id' });
  assert.equal((html.match(/<p class="dw-text">/g) || []).length, 8);
  for (const text of paragraphs.slice(0, 7)) assert.ok(html.includes(text), text);
  assert.match(html, /&lt;script&gt;not markup&lt;\/script&gt; &amp; Report/);
  assert.doesNotMatch(html, /<script>|hidden-host-id|"paragraphs"/);
  assert.equal(openingDispatchHTML({}), '');
});

test('opening dispatch destination resolves the current own Host from its id', () => {
  State.unitsData = [{ id: 'opening-host', q: 17, r: 23 }];
  State.foreignUnitData = [];
  assert.deepEqual(resolveDestination('OpeningDispatch', { host_id: 'opening-host' }), { q: 17, r: 23 });
  assert.equal(resolveDestination('OpeningDispatch', { host_id: 'old-host' }), null);
});

test('stalled dispatch shows owned sealed order with escaped text', () => {
 const html=passageStalledBlockHTML({messenger_id:'m',own_port:true,order_summary:'Third Spearmen: March to (37,21); mission: colonize <bad>'});
 assert.match(html,/Sealed order: Third Spearmen: March to \(37,21\)/);
 assert.match(html,/mission: colonize &lt;bad&gt;/);
 assert.doesNotMatch(html,/<bad>/);
});

test('PassageStalled prints the server explanation, one source for web and keryx', () => {
  const html = passageStalledBlockHTML({ messenger_id: 'm', own_port: false, eligible_ships: [],
    explanation: "Your runner is waiting in Amnissos: no ship has sailed from there toward Kydonia. Any ship that does — yours or another Wanax's — takes it aboard by itself." });
  assert.ok(html.includes('Amnissos') && html.includes('takes it aboard by itself'), html);
  assert.ok(!html.includes('No eligible ship of yours'), 'the old client-side line must not contradict the explanation');
});

test('A.1: letter and trade-offer dispatches name the Wanax that Reply opens (Timothy 2026-10-23)', () => {
  assert.equal(letterPersonFor('MessengerArrival', { sender_id: 'p-writer', q: 1, r: 1 }), 'p-writer');
  assert.equal(letterPersonFor('MessengerReturned', { counterpart_wanax_id: 'p-them' }), 'p-them');
  assert.equal(letterPersonFor('OfferAccepted', { counterpart_wanax_id: 'p-them' }), 'p-them');
  assert.equal(letterPersonFor('OfferDeclined', { counterpart_wanax_id: 'p-them' }), 'p-them');
  // Old archive rows carry no id: they keep Take me there.
  assert.equal(letterPersonFor('MessengerArrival', { q: 1, r: 1 }), '');
  // Other kinds never become Reply, even with a stray id.
  assert.equal(letterPersonFor('UnitArrived', { sender_id: 'p-x' }), '');
});

test('a delayed route lookup cannot replace a newer dispatch or reopen a closed window',async()=>{
 const {openDispatchWindow,closeDispatchWindow}=await import('./dispatch_window.js');
 const saved={document:globalThis.document,window:globalThis.window,localStorage:globalThis.localStorage,fetch:globalThis.fetch};
 const classes=new Set();const element=()=>({innerHTML:'',style:{},classList:{add:k=>classes.add(k),remove:k=>classes.delete(k),toggle(){}},addEventListener(){}});
 const overlay=element(),body=element(),node=element();
 globalThis.document={body:node,getElementById:id=>id==='dispatch-window-overlay'?overlay:id==='dw-body'?body:id==='net-status'?null:node};
 globalThis.window={matchMedia:()=>({matches:false})};globalThis.localStorage={getItem:()=>null};
 State.WORLD_ID='world';State.provinceData=[{id:'city',own:true,name:'New city',q:0,r:0}];
 let finish;
 globalThis.fetch=async url=>String(url).includes('standing-orders')?new Promise(resolve=>{finish=()=>resolve(new Response('[{"id":"route"}]',{status:200}));}):new Response('{"muted_kinds":[]}',{status:200});
 try{
  const pending=openDispatchWindow('StandingOrderPaused',{standing_order_id:'route'});
  await openDispatchWindow('BuildComplete',{settlement_id:'city',building_type:'lumbermill',settlement_name:'New city'});
  const newest=body.innerHTML;finish();await pending;assert.equal(body.innerHTML,newest,'stale route overwrote the newer completion');
  const closed=openDispatchWindow('StandingOrderPaused',{standing_order_id:'route'});closeDispatchWindow();finish();await closed;
  assert.equal(classes.has('open'),false,'late route lookup reopened dismissed dispatch');
 }finally{closeDispatchWindow();Object.assign(globalThis,saved);}
});
