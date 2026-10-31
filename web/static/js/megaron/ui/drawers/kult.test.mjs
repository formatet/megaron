import test from 'node:test';
import assert from 'node:assert/strict';

// kult.js pulls in ui/misc.js (renderLockedActions), which runs
// `document.addEventListener(...)` at module top level (music-player
// autostart) — same problem documented in cargo.test.mjs/economy.test.mjs.
// Stub the one global it touches, then dynamically import.
globalThis.document ??= { addEventListener() {} };
const { cultLevelLabel, prayerStatusHTML, templeOfferingsHTML, prayerResultText,kultCityForID } = await import('./kult.js');

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
 assert.match(html,/on cooldown.*24 game days/);assert.doesNotMatch(html,/Offer|24m|not affordable/);
});
test('Alpha: active gift shows its remaining game days and cannot be offered again', () => {
 const html=prayerStatusHTML({id:'minoan_velchanos_green_shoots',affordable:false,active_remaining_game_days:25,cooldown_remaining_game_days:25});
 assert.match(html,/active.*25 game days/);assert.doesNotMatch(html,/Offer|not affordable/);
});
test('Alpha: affordable available prayer still exposes the real cast action', () => {
 assert.match(prayerStatusHTML({id:'minoan_potnia_vision_v2',affordable:true}),/okRite\('minoan_potnia_vision_v2'\).*Offer/);
});

test('Alpha B: unused departure gift remains pending after god wait expires', () => {
  const html = prayerStatusHTML({ id: 'minoan_asterion_thread', departure_pending: true, affordable: false, cooldown_remaining_game_days: 0 });
  assert.match(html, /awaiting next departure/);
  assert.doesNotMatch(html, /Offer|not affordable|active|game days|expires/);
});

test('Alpha B: pending ship gift shows god wait separately without a gift duration', () => {
  const html = prayerStatusHTML({ id: 'minoan_stillness_passage', departure_pending: true, affordable: false, cooldown_remaining_game_days: 12 });
  assert.match(html, /awaiting next departure; god waits 12 game days/);
  assert.doesNotMatch(html, /Offer|not affordable|active|left|expires/);
});

test('Alpha B: consumed departure gift waits for the god before another Offer', () => {
  const html = prayerStatusHTML({ id: 'minoan_asterion_thread', departure_pending: false, affordable: false, cooldown_remaining_game_days: 7 });
  assert.match(html, /on cooldown.*7 game days/);
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
  assert.match(html, /reference 0\.375 grain per game day/);
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
  assert.match(templeOfferingsHTML([]), /No temples</);
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
  const pd={kharis_population:2500,kharis_population_decay:.25,kharis_city_count:5,kharis_base_decay:0.1,kharis_city_decay:1,kharis_daily_decay:1.1,
    kharis_net_known:true,kharis_net_per_tick:-1.1,kharis_per_tick:0,temple_offers:[],available_prayers:[]};
  globalThis.fetch=async url=>{
    calls.push(url);
    if(url.endsWith('/cult'))return new Response(JSON.stringify({kharis:pd,temples:[],prayers:[],unbuilt_temples:[]}));
    if(url.endsWith('/actions'))return new Response('[]');
    assert.fail('Unexpected API request '+url);
  };
  await loadKultDrawer();
  assert.match(body.innerHTML,/−1.1 \/ game day/);
  assert.match(body.innerHTML,/-1.1 kharis per game day/);
  assert.match(body.innerHTML,/No temples/);
  assert.doesNotMatch(body.innerHTML,/Passive|144|holds for/);
  assert.equal(calls.length,2);
});

test('ORDER15: each temple names its own offering row; prayer shows the temple it needs', () => {
  const html = templeOfferingsHTML([
    { ...templeFixture, name: 'Phaistos', temple: 'Temple of Potnia' },
    { ...templeFixture, name: 'Phaistos', temple: 'Temple of Velchanos', fed: false, offering: {} },
  ]);
  assert.match(html, /Phaistos — Temple of Potnia: /);
  assert.match(html, /Phaistos — Temple of Velchanos: /);
  assert.equal((html.match(/Next gift/g) || []).length, 2);
  assert.match(prayerStatusHTML({ id: 'x', affordable: true, unavailable_reason: 'no Temple of Velchanos' }), /no Temple of Velchanos/);
  assert.doesNotMatch(prayerStatusHTML({ id: 'x', affordable: true, unavailable_reason: 'no Temple of Velchanos' }), /Offer/);
});

test('everyday result has no synthetic silent answer; legacy and recorded history retain theirs', () => {
 assert.equal(prayerResultText({success: true}, {god_id:'potnia'}), 'The gods answered!');
 assert.equal(prayerResultText({success: false}, {god_id:'potnia'}), 'The prayer could not be completed.');
 assert.equal(prayerResultText({success: false}, {}), 'The gods are silent.');
 assert.equal(prayerResultText({message:'The gods were silent.'}, {god_id:'potnia'}), 'The gods were silent.');
});

test('concrete temple and kharis blockers stay visible during a god wait',()=>{
 for(const reason of ['no Temple of Potnia','insufficient kharis']) {
  const html=prayerStatusHTML({id:'minoan_potnia_vision_v2',affordable:false,cooldown_remaining_game_days:24,unavailable_reason:reason});
  assert.ok(html.includes(reason));assert.doesNotMatch(html,/on cooldown|okRite/);
 }
});

test('preparation is reachable without the traditional recipe, not a promise that payment will succeed',()=>{
 const html=prayerStatusHTML({id:'minoan_potnia_vision_v2',affordable:false,can_prepare_offering:true,unavailable_reason:'insufficient offering'});
 assert.match(html,/okRite\('minoan_potnia_vision_v2'\).*Compose offering/);
 assert.match(html,/Gift unavailable/);assert.doesNotMatch(html,/traditional gift/);
});

test('dispatch cult city selection is exact and cannot fall back to the capital for another city',()=>{
 const capital={id:'cap',settlement_id:'s1',own:true},target={id:'target',settlement_id:'s2',own:true};
 const rows=[capital,target,{id:'foreign',own:false}];
 assert.equal(kultCityForID(rows,'target',capital),target);
 assert.equal(kultCityForID(rows,'gone',capital),null);
 assert.equal(kultCityForID(rows,'foreign',capital),null);
 assert.equal(kultCityForID(rows,null,capital),capital);
});

test('same god in two cities: offering from second row posts only to that city',async t=>{
 const {State}=await import('../../state.js');const {loadKultDrawer,bringOffering}=await import('./kult.js');
 const state={WORLD_ID:State.WORLD_ID};const old={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};
 t.after(()=>{Object.assign(State,state);Object.assign(globalThis,old);});
 const result={};const body={innerHTML:'',querySelector:()=>result};
 globalThis.document={getElementById:id=>id==='kult-body'?body:null,addEventListener(){}};
 globalThis.localStorage={getItem:()=>null};State.WORLD_ID='realm-routing';const posts=[];
 const prayers=['one','two'].map(id=>({id:'same-prayer',settlement_id:id,province_id:'p-'+id,city_name:id,has_temple:true,affordable:true,can_prepare_offering:true}));
 globalThis.fetch=async(url,options={})=>{
  if(url.endsWith('/cult'))return new Response(JSON.stringify({kharis:{},temples:[],prayers,unbuilt_temples:[]}));
  if(url.endsWith('/actions'))return new Response('[]');
  if(options.method==='POST'){posts.push([url,JSON.parse(options.body)]);return new Response(JSON.stringify({success:true,message:'Answered'}));}
  assert.fail('Unexpected '+url);
 };
 await loadKultDrawer();await bringOffering('two__same-prayer');
 assert.deepEqual(posts,[['/api/v1/worlds/realm-routing/settlements/two/rite',{prayer:'same-prayer'}]]);
});

test('Bönegrund: a resting temple names itself, its rest, and the pause for want of servants', () => {
  const p = { id: 'minoan_potnia_walls', affordable: false, temple: 'Temple of Potnia', cooldown_remaining_game_days: 3, rest_remaining_game_days: 3, rest_paused: true, temple_staffed: 2, temple_capacity: 4 };
  const html = prayerStatusHTML(p);
  assert.match(html, /Temple of Potnia rests 3 game days — paused: 2 of 4 servants/);
  assert.doesNotMatch(html, /Offer|on cooldown/);
  assert.match(prayerStatusHTML({ ...p, rest_paused: false }), /Temple of Potnia rests 3 game days<\/span>/);
});
