import test from 'node:test';
import assert from 'node:assert/strict';

// kult.js pulls in ui/misc.js (renderLockedActions), which runs
// `document.addEventListener(...)` at module top level (music-player
// autostart) — same problem documented in cargo.test.mjs/economy.test.mjs.
// Stub the one global it touches, then dynamically import.
globalThis.document ??= { addEventListener() {} };
const { cultLevelLabel, prayerStatusHTML, templeOfferingsHTML } = await import('./kult.js');

// The server never runs a five-tier cult-level model (forsummad/enkel/vardig/
// praktfull/overdadig) — internal/kharis/tick.go's deriveMood "replaces
// player-set cult_level" and writes ONE of four mood strings into the same
// column, in Swedish: tveksam/vardig/overdadig/vredgad (plus 'enkel' as the
// settlement.go fallback before a player's first daily tick has ever run).
// 'forsummad' and 'praktfull' are dead values the server can never send.

test('AK1: vredgad (Wrathful mood) renders an English label, not the raw Swedish string', () => {
  assert.equal(cultLevelLabel('vredgad'), 'Wrathful');
});

test('AK1b: tveksam (Suspicious mood) renders an English label, not the raw Swedish string', () => {
  assert.equal(cultLevelLabel('tveksam'), 'Suspicious');
});

test('AK2: vardig and overdadig — already-correct-looking values — still translate correctly', () => {
  assert.equal(cultLevelLabel('vardig'), 'Indifferent');
  assert.equal(cultLevelLabel('overdadig'), 'Favorable');
});

test('AK3: enkel (pre-first-tick fallback) gets an honest label, not a fake tier name', () => {
  assert.equal(cultLevelLabel('enkel'), 'Not yet assessed');
});

test('AK4: a value the server never sends still falls back to itself, not a fabricated label', () => {
  assert.equal(cultLevelLabel('some_unknown_value'), 'some_unknown_value');
});


test('Alpha: realm god waiting time uses game days and blocks Offer', () => {
 const html=prayerStatusHTML({id:'minoan_potnia_walls',affordable:false,cooldown_remaining_game_days:24});
 assert.match(html,/on cooldown.*24 days/);assert.doesNotMatch(html,/Offer|24m|not affordable/);
});
test('Alpha: active gift shows its remaining days and cannot be offered again', () => {
 const html=prayerStatusHTML({id:'minoan_velchanos_green_shoots',affordable:false,active_remaining_game_days:25,cooldown_remaining_game_days:25});
 assert.match(html,/active.*25 days/);assert.doesNotMatch(html,/Offer|not affordable/);
});
test('Alpha: affordable available prayer still exposes the real cast action', () => {
 assert.match(prayerStatusHTML({id:'minoan_potnia_vision_v2',affordable:true}),/okRite\('minoan_potnia_vision_v2'\).*Offer/);
});

test('Alpha B: unused departure gift remains pending after god wait expires', () => {
  const html = prayerStatusHTML({ id: 'minoan_asterion_thread', departure_pending: true, affordable: false, cooldown_remaining_game_days: 0 });
  assert.match(html, /awaiting next departure/);
  assert.doesNotMatch(html, /Offer|not affordable|active|days|expires/);
});

test('Alpha B: pending ship gift shows god wait separately without a gift duration', () => {
  const html = prayerStatusHTML({ id: 'minoan_stillness_passage', departure_pending: true, affordable: false, cooldown_remaining_game_days: 12 });
  assert.match(html, /awaiting next departure; god waits 12 days/);
  assert.doesNotMatch(html, /Offer|not affordable|active|left|expires/);
});

test('Alpha B: consumed departure gift waits for the god before another Offer', () => {
  const html = prayerStatusHTML({ id: 'minoan_asterion_thread', departure_pending: false, affordable: false, cooldown_remaining_game_days: 7 });
  assert.match(html, /on cooldown.*7 days/);
  assert.doesNotMatch(html, /Offer|awaiting next departure|not affordable/);
});


const templeFixture = {
  name: 'Phaistos', required_goods: { grain: 0.375 },
  alternatives: { silver: 0.5, timber: 0.125, fish: 0.75 },
  stock: { grain: 0.1, fish: 0.55, timber: 0, silver: 3 },
  offering: { fish: 0.55, grain: 0.1 }, fed: true,
};

test('temple rows render the server reference and mixed next gift without repricing', () => {
  const html = templeOfferingsHTML([templeFixture]);
  assert.match(html, /Phaistos: <\/span> <span/);
  assert.match(html, /reference 0\.375 grain per day/);
  assert.match(html, /Alternatives: 0\.75 fish or 0\.125 timber or 0\.5 silver/);
  assert.match(html, /Stock: 0\.1 grain, 0\.55 fish, 0 timber, 3 silver/);
  assert.match(html, /Next gift: 0\.1 grain \+ 0\.55 fish/);
  assert.match(html, /✓/);
  assert.doesNotMatch(html, /oil|wine/);
});

test('quarter-grain reference and smaller silver debit remain visible', () => {
  const html = templeOfferingsHTML([{ ...templeFixture, required_goods: { grain: .25 }, offering: { silver: .0125 } }]);
  assert.match(html, /reference 0\.25 grain/);
  assert.match(html, /Next gift: 0\.0125 silver/);
});

test('unfunded temple shows all-or-nothing outcome and escapes the city name', () => {
  const html = templeOfferingsHTML([{ ...templeFixture, name: '<Knossos>', fed: false, offering: {} }]);
  assert.match(html, /&lt;Knossos&gt;/);
  assert.match(html, /Next gift: unfunded \(nothing taken\)/);
  assert.match(html, /✗/);
  assert.doesNotMatch(html, /✓/);
});

test('realm without temples explains the missing gain', () => {
  assert.match(templeOfferingsHTML([]), /No temples — kharis will not climb/);
});

test('Kult controller renders the realm breakdown and templeless net from the province API', async t => {
  const {State}=await import('../../state.js');
  const {loadKultDrawer}=await import('./kult.js');
  const state={WORLD_ID:State.WORLD_ID,MY_SETTLEMENT_ID:State.MY_SETTLEMENT_ID,provinceData:State.provinceData};
  const globals={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};
  t.after(()=>{Object.assign(State,state);for(const [key,value] of Object.entries(globals))value===undefined?delete globalThis[key]:globalThis[key]=value;});
  const body={innerHTML:''};
  globalThis.document={getElementById:id=>id==='kult-body'?body:null,addEventListener(){}};
  globalThis.localStorage={getItem:()=>null};
  Object.assign(State,{WORLD_ID:'realm',MY_SETTLEMENT_ID:'capital',provinceData:[{id:'province',own:true,is_capital:true}]});
  const calls=[];
  const pd={kharis_city_count:5,kharis_base_decay:0.1,kharis_city_decay:1,kharis_daily_decay:1.1,
    kharis_net_known:true,kharis_net_per_tick:-1.1,kharis_per_tick:0,temple_offers:[],available_prayers:[]};
  globalThis.fetch=async url=>{
    calls.push(url);
    if(url.endsWith('/settlements/capital'))return new Response(JSON.stringify({divine_mood:'Indifferent',cult_level:'vardig'}));
    if(url.endsWith('/provinces/province'))return new Response(JSON.stringify({settlement:pd}));
    if(url.endsWith('/actions'))return new Response('[]');
    assert.fail('Unexpected API request '+url);
  };
  await loadKultDrawer();
  assert.match(body.innerHTML,/0.1 base \+ 5 cities cost 1.0 per day = 1.1 total/);
  assert.match(body.innerHTML,/-1.1 kharis per day/);
  assert.match(body.innerHTML,/No temples/);
  assert.doesNotMatch(body.innerHTML,/Passive|144|holds for/);
  assert.equal(calls.length,3);
});
