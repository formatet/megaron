import test from 'node:test';
import assert from 'node:assert/strict';

// The march menu grouped units by type + origin and labelled each row with the
// TYPE, so a player ordering one of several identical spearmen had nothing to
// go on but the hex coordinates (Timothy 2026-08-04: "därför räknar Timothy
// hexar för att veta vem han beordrar"). groupMarchUnits/marchGroupLabelHTML/
// marchGroupNamesHTML are the pure half of the fix.
//
// marchctx.js touches document.getElementById at module top level and pulls in
// render/map.js, so — same trick as cargo.test.mjs — stub the globals its
// import graph reaches, THEN import dynamically. A static import would evaluate
// that graph before any test body runs.
const noopEl = new Proxy({}, {
  get: (_t, k) => (k === 'style' ? {} : (k === 'value' ? '' : () => noopEl)),
  set: () => true,
});
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document ??= {
  addEventListener() {},
  getElementById: () => noopEl,
  createElement: () => noopEl,
  querySelector: () => noopEl,
  querySelectorAll: () => [],
  body: noopEl,
};
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };

const { groupMarchUnits, marchGroupLabelHTML, marchGroupNamesHTML, resolveMarchIntent, exploreRowVisible, hiketeiaRefusalHTML } =
  await import('./marchctx.js');

const spearman = (id, ordinal, q, r) => ({
  id,
  type: 'spearman',
  display_name: ordinal + ' Spearmen of Knossos',
  settlement_id: null,
  q, r,
});

test('AK1: a group of one wears its own name, not the bare type', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  assert.equal(groups.length, 1);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /First Spearmen of Knossos/,
    'a lone unit must be named — this is the row that used to read just "Spearmen"');
});

test('AK2: the field unit keeps its (q,r) tag — the name says who supports it, the tag says where it stands', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /\(12,7\)/);
});

test('AK3: a garrisoned unit does not say its town twice', () => {
  const u = { id: 'u1', type: 'spearman', display_name: 'First Spearmen of Knossos', settlement_id: 's1' };
  const groups = groupMarchUnits([u], [{ settlement_id: 's1', name: 'Knossos' }]);
  const html = marchGroupLabelHTML(groups[0]);
  assert.match(html, /First Spearmen of Knossos/);
  assert.equal(html.match(/Knossos/g).length, 1, 'the "· Knossos" tag repeats the name and must be dropped');
});

test('AK4: a group of several keeps the type label and lists its members in send order', () => {
  const units = [
    spearman('u1', 'First', 12, 7),
    spearman('u2', 'Second', 12, 7),
    spearman('u3', 'Third', 12, 7),
  ];
  const groups = groupMarchUnits(units, []);
  assert.equal(groups.length, 1, 'identical units at one place stay ONE group — the counter is the affordance');
  assert.equal(groups[0].ids.length, 3);

  const names = marchGroupNamesHTML(groups[0]);
  assert.match(names, /1\. First Spearmen of Knossos/);
  assert.match(names, /2\. Second Spearmen of Knossos/);
  assert.match(names, /3\. Third Spearmen of Knossos/);

  // names[] must track ids[] index for index, because sendMarch sends ids[0..n-1].
  assert.deepEqual(groups[0].ids, ['u1', 'u2', 'u3']);
  assert.deepEqual(groups[0].names, [
    'First Spearmen of Knossos', 'Second Spearmen of Knossos', 'Third Spearmen of Knossos',
  ]);
});

test('AK5: a lone unit gets no member list — the label already names it', () => {
  const groups = groupMarchUnits([spearman('u1', 'First', 12, 7)], []);
  assert.equal(marchGroupNamesHTML(groups[0]), '');
});

test('AK6: units at different places stay different groups', () => {
  const groups = groupMarchUnits(
    [spearman('u1', 'First', 12, 7), spearman('u2', 'Second', 3, 3)],
    [],
  );
  assert.equal(groups.length, 2);
});

test('AK7: a missing display_name falls back to the type label, never a blank row', () => {
  const u = { id: 'u1', type: 'spearman', q: 1, r: 1 };
  const groups = groupMarchUnits([u], []);
  assert.ok(groups[0].names[0], 'name must not be empty');
  assert.match(marchGroupLabelHTML(groups[0]), /\w/);
});

test('AK8: a hostile unit name is escaped, not injected', () => {
  const u = { id: 'u1', type: 'spearman', display_name: '<img src=x onerror=alert(1)>', q: 1, r: 1 };
  const groups = groupMarchUnits([u], []);
  assert.doesNotMatch(marchGroupLabelHTML(groups[0]), /<img/);
});

// resolveMarchIntent — found 2026-09-24: the server forbids a plain march
// onto land no man of yours has ever seen, UNLESS intent=explore
// (march_start.go's FOW rule). The web only ever sent explore for sea
// targets, so a web player could not explore unknown land with a land unit.
// This is the pure decision the fix hangs on: sea and unseen-land are always
// explore (the FOW rule accepts nothing else on unseen ground, and no
// separate recall order exists for ships); known land is plain march unless
// the player opted in.
test('MI1: sea target is always explore, regardless of the checkbox', () => {
  assert.equal(resolveMarchIntent({ isSea: true, known: true }, false), 'explore');
  assert.equal(resolveMarchIntent({ isSea: true, known: true }, true), 'explore');
});

test('MI2: an unseen (fog) land target is always explore — the only intent the server FOW rule allows there', () => {
  assert.equal(resolveMarchIntent({ isSea: false, known: false }, false), 'explore');
});

test('MI3: a known land target is a plain march by default', () => {
  assert.equal(resolveMarchIntent({ isSea: false, known: true }, false), '');
});

test('MI4: a known land target explores when the player opts in', () => {
  assert.equal(resolveMarchIntent({ isSea: false, known: true }, true), 'explore');
});

test('MI5: exploreRowVisible offers the checkbox on land, never on sea or a settlement', () => {
  assert.equal(exploreRowVisible({ isSea: false, isSettlement: false }), true);
  assert.equal(exploreRowVisible({ isSea: true, isSettlement: false }), false);
  assert.equal(exploreRowVisible({ isSea: false, isSettlement: true }), false);
});

// Timothy 2026-09-25: "it doesn't seem possible to give orders to units that
// have already been given orders — that is wrong, they must be reachable by
// orders." The right-click march menu used to filter marching units out
// entirely; marchCtxOrderMode is the pure decision (eligible? march or
// redirect?) that now drives both that filter and the group's send target.
const { marchCtxOrderMode } = await import('./marchctx.js');

test('AK9: a garrisoned/positioned unit is eligible to march', () => {
  assert.equal(marchCtxOrderMode({ status: 'garrison', deployable: true }), 'march');
  assert.equal(marchCtxOrderMode({ status: 'positioned', deployable: true }), 'march');
});

test('AK10: a marching unit is eligible too — but for redirect, not a fresh march', () => {
  assert.equal(marchCtxOrderMode({ status: 'marching', deployable: true }), 'redirect');
});

test('R3 (megaron_plan_skeppsuppdrag_landsatt.md): a marching SHIP is not eligible for redirect — it takes no orders at sea', () => {
  assert.equal(marchCtxOrderMode({ status: 'marching', deployable: true, category: 'naval' }), null);
});

test('AK11: a fortified unit stays ineligible (server blocks fresh march on it)', () => {
  assert.equal(marchCtxOrderMode({ status: 'garrison', deployable: true, stance: 'fortify' }), null);
});

test('AK12: a still-forming/training unit stays ineligible even while nominally marching-shaped', () => {
  assert.equal(marchCtxOrderMode({ status: 'forming', deployable: false }), null);
  assert.equal(marchCtxOrderMode({ status: 'marching', deployable: false }), null);
});

test('AK13: a unit embarked/disbanded/other status is ineligible', () => {
  assert.equal(marchCtxOrderMode({ status: 'embarked', deployable: true }), null);
});

test('AK14: marching and garrisoned units of the same type+hex stay in SEPARATE groups — a redirect send must never merge with a fresh-march send', () => {
  const units = [
    { id: 'u1', type: 'spearman', status: 'garrison', deployable: true, q: 5, r: 5, display_name: 'First Spearmen' },
    { id: 'u2', type: 'spearman', status: 'marching', deployable: true, q: 5, r: 5, display_name: 'Second Spearmen' },
  ];
  const groups = groupMarchUnits(units, []);
  assert.equal(groups.length, 2, 'march and redirect groups must not merge even at the same (q,r)');
  const byMode = Object.fromEntries(groups.map(g => [g.mode, g]));
  assert.deepEqual(byMode.march.ids, ['u1']);
  assert.deepEqual(byMode.redirect.ids, ['u2']);
});

test('AK15: a redirect group is marked in its label — the player must see this send goes to a Runner', () => {
  const u = { id: 'u1', type: 'spearman', status: 'marching', deployable: true, q: 5, r: 5, display_name: 'First Spearmen' };
  const groups = groupMarchUnits([u], []);
  assert.match(marchGroupLabelHTML(groups[0]), /redirect by messenger/);
});

const menu = await import('./marchctx.js');
const ready = (id, extra = {}) => ({ id, type: 'spearman', category: 'land', status: 'garrison', deployable: true, ...extra });
test('H: defaults send all available units in the chosen first group, other groups stay unselected', () => {
  const groups = [{ ids: ['a', 'b', 'c'] }, { ids: ['d', 'e'] }];
  assert.deepEqual(groups.map((g, i) => menu.marchDefaultQuantity(g, i)), [3, 0], 'all units of the first group must be ready for a single click');
});
test('H: War target selection lists only the selected unit; normal map menu keeps all eligible units', () => {
  const units = [ready('a'), ready('b'), ready('no', { deployable: false })];
  const dest = { known: true, isSea: false };
  assert.deepEqual(menu.marchEligibleUnits(units, dest, 'b').map(u => u.id), ['b'], 'War must never march another unit');
  assert.deepEqual(menu.marchEligibleUnits(units, dest).map(u => u.id), ['a', 'b']);
});
test('H: a loaded ship can land on known land, while empty and marching ships cannot', () => {
  const units = [ready('land'), ready('loaded', { category: 'naval', cargo_unit_id: 'cargo' }), ready('empty', { category: 'naval' }), ready('sailing', { category: 'naval', cargo_unit_id: 'cargo', status: 'marching' })];
  assert.deepEqual(menu.marchEligibleUnits(units, { known: true, isSea: false }, 'loaded').map(u => u.id), ['loaded']);
  assert.deepEqual(menu.marchEligibleUnits(units, { known: true, isSea: false }, 'empty'), []);
  assert.deepEqual(menu.marchEligibleUnits(units, { known: true, isSea: false }, 'sailing'), []);
  assert.deepEqual(menu.marchEligibleUnits(units, { known: false, isSea: false }, 'loaded').map(u => u.id), ['loaded']);
  assert.deepEqual(menu.marchEligibleUnits(units, { known: true, isSea: false }).map(u => u.id), ['land']);
});

test('H: landing preserves cargo-colonize/name and omits naval stance; explore and plain land retain options', () => {
  assert.deepEqual(menu.marchIntentOptions({known:true,isSea:false}, {landing:true,colonize:true,name:'Nostos',stance:'storm'}), {intent:'land',cargo_intent:'colonize',name:'Nostos'});
  assert.deepEqual(menu.marchIntentOptions({known:true,isSea:false}, {colonize:true,name:'Nostos',stance:'sentry'}), {intent:'colonize',name:'Nostos',stance:'sentry'});
  assert.deepEqual(menu.marchIntentOptions({known:false,isSea:false}), {intent:'explore'});
  assert.deepEqual(menu.marchIntentOptions({known:true,isSea:true}, {stance:'storm'}), {intent:'explore'});
  assert.deepEqual(menu.marchIntentOptions({known:true,isSea:false}, {stance:'sentry'}), {stance:'sentry'});
});

 test('H: colony forecast keeps production and food duration in game day words', () => {
  const html = menu.renderColonizePreviewHTML({catchment:[{known:true,terrain:'plains'}],grain:{base_per_tick:3,est_net_per_tick:-2,ticks_until_empty:4,seed:8,with_farm_per_tick:7},goods:{fish:2}});
  assert.match(html, /lasts about 4 game days/);
  assert.match(html, /grain 3\.0 \+ fish 0\.0 − consumption 5\.0 per game day/);
  assert.match(html, /fish about 2 per game day/);
  assert.doesNotMatch(html, /\/game day|\d+ ticks/);
});

test('forecast shows the server harbour note and nothing when absent', () => {
  const base = {catchment:[{known:true,terrain:'plains'}],grain:{base_per_tick:3,est_net_per_tick:1,seed:8,with_farm_per_tick:3},goods:{}};
  const inland = menu.renderColonizePreviewHTML({...base, harbour_note:'Inland: a city here can never build a harbour or ships'});
  assert.match(inland, /can never build a harbour or ships/);
  assert.doesNotMatch(menu.renderColonizePreviewHTML(base), /harbour/);
});

test('river march: real map destination offers naval only, ford offers both categories', async () => {
  const { destFromHex } = await import('../render/map.js');
  const units = [ready('land'), ready('ship', { category: 'naval' }), ready('sailing', { category: 'naval', status: 'marching' })];
  for (const terrain of ['river', 'coastal_sea', 'deep_sea']) {
    const dest = destFromHex({q:6,r:67}, {terrain}, null);
    assert.equal(dest.isSea, true, terrain);
    assert.deepEqual(menu.marchEligibleUnits(units, dest).map(u => u.id), ['ship'], terrain);
  }
  const ford = destFromHex({q:6,r:67}, {terrain:'river_ford'}, null);
  assert.equal(ford.isSea, false);
  assert.deepEqual(menu.marchEligibleUnits(units, ford).map(u => u.id), ['land', 'ship']);
  assert.deepEqual(menu.marchUnitOptions(ford, 'land', {stance:'sentry'}), {stance:'sentry'});
  assert.deepEqual(menu.marchUnitOptions(ford, 'naval', {intent:'colonize',name:'Nostos',stance:'sentry'}), {intent:'explore'});
  const plain = destFromHex({q:6,r:67}, {terrain:'plains'}, null);
  assert.deepEqual(menu.marchEligibleUnits(units, plain).map(u => u.id), ['land']);
});

test('ford send uses the same per-unit mission as preview, retaining target and expedition duration', async () => {
  const { State } = await import('../state.js');
  const originalFetch = globalThis.fetch;
  const originalGet = document.getElementById;
  const before = { dest:State.marchCtxDest, groups:State.marchCtxGroups, units:State.marchCtxUnits, id:State.marchCtxUnitID, world:State.WORLD_ID };
  const controls = new Map();
  const control = id => { if (!controls.has(id)) controls.set(id, {value:'',checked:false,style:{display:'block'},classList:{toggle(){}},disabled:false}); return controls.get(id); };
  const calls = [];
  try {
    document.getElementById = control;
    control('mg-0').value='1'; control('mg-1').value='1';
    Object.assign(control('mctx-ticks'), {value:'12',min:'2',max:'96',disabled:false});
    control('mctx-stance').value='sentry';
    State.WORLD_ID='ford-proof';State.marchCtxUnitID=null;
    State.marchCtxDest={q:6,r:67,terrain:'river_ford',known:true,isSea:false};
    State.marchCtxGroups=[{ids:['land'],mode:'march',category:'land'},{ids:['ship'],mode:'march',category:'naval'}];
    State.marchCtxUnits=[ready('land'),ready('ship',{category:'naval'})];
    globalThis.fetch=async (url, opts = {}) => {
      if (opts.method === 'POST') calls.push({url,body:JSON.parse(opts.body)});
      // Refuse after observing the submitted bodies: no world refresh or audio.
      return {ok:false,status:422,headers:{get:()=>null},json:async()=>({error:'proof refusal'})};
    };
    await menu.sendMarch();
    await new Promise(resolve => setTimeout(resolve, 0));
    assert.equal(calls.length,2);
    assert.deepEqual(calls[0].body,{target_q:6,target_r:67,stance:'sentry'});
    assert.deepEqual(calls[1].body,{target_q:6,target_r:67,intent:'explore',ticks:12});
  } finally {
    menu.closeMarchCtx();
    globalThis.fetch=originalFetch;document.getElementById=originalGet;
    Object.assign(State,{marchCtxDest:before.dest,marchCtxGroups:before.groups,marchCtxUnits:before.units,marchCtxUnitID:before.id,WORLD_ID:before.world});
  }
});

test('playtest: coastal forecast uses full food demand without a loyalty reserve warning', () => {
  const html = menu.renderColonizePreviewHTML({grain: {base_per_tick: 0.5, fish_per_tick: 5, est_net_per_tick: 0, consumption_per_tick: 5, food_net_per_tick: 0.5, seed: 0, loyalty_warning: 'Obsolete loyalty warning'}});
  assert.match(html, /grain 0\.5 \+ fish 5\.0 − consumption 5\.0/);
  assert.match(html, /net \+0\.5/);
  assert.doesNotMatch(html, /Obsolete loyalty warning/);
});

test('founding carries the authoritative wood warning without inventing a local rule', () => {
 const html=menu.renderColonizePreviewHTML({timber_warning:'No timber on known ground — scout the rest',grain:{base_per_tick:5,consumption_per_tick:5,food_net_per_tick:0}});
 assert.match(html,/No timber on known ground — scout the rest/);
});


test('Host summary exposes the wood warning before the closed Details section', async () => {
 const {hostForecastSummaryHTML}=await import('../render/map.js');
 assert.match(hostForecastSummaryHTML({timber_warning:'No timber within reach',goods:{}}),/No timber within reach/);
});

test('fish-fed founding forecast does not invent an empty-grain loyalty warning', () => {
 const html=menu.renderColonizePreviewHTML({grain:{base_per_tick:0,fish_per_tick:5,consumption_per_tick:5,food_net_per_tick:0,est_net_per_tick:0,seed:0}});
 assert.match(html,/fish 5.0/);assert.doesNotMatch(html,/loyalty can fall|No grain reserve/);
});


test('AI forecast avoids dust horizons and identifies the farm already included', () => {
 const html=menu.renderColonizePreviewHTML({grain:{base_per_tick:5,food_net_per_tick:-1e-12,consumption_per_tick:5,ticks_until_empty:1e18,seed:1200}});
 assert.doesNotMatch(html,/-0.0|1000000000000000000|lasts about/);assert.match(html,/about even/);
 const farm=menu.renderColonizePreviewHTML({grain:{base_per_tick:4.67,food_net_per_tick:-0.33,consumption_per_tick:5,with_farm_per_tick:4.67,with_farm_food_net_per_tick:-0.33,farm_possible:true,starter_farm_included:true}});
 assert.match(farm,/already included/);assert.doesNotMatch(farm,/no farmland/);assert.match(farm,/-0.3/);
});

test('hiketeia: a refused march shows the warning with March anyway; other refusals do not', () => {
  const html = hiketeiaRefusalHTML([{ data: { hiketeia: true, error: 'These are the survivors of a fallen Wanax.' } }]);
  assert.match(html, /survivors of a fallen Wanax/);
  assert.match(html, /March anyway/);
  assert.equal(hiketeiaRefusalHTML([{ data: { error: 'too far' } }]), '');
});

test('own coastal city offers empty and laden ships in the map menu and War target picker',()=>{
 const units=[ready('land'),ready('empty',{category:'naval'}),ready('laden',{category:'naval',cargo_unit_id:'cargo'}),ready('sailing',{category:'naval',status:'marching'})];
 const city={q:4,r:2,known:true,isSea:false,isSettlement:true,own:true,coastal:true};
 assert.deepEqual(menu.marchEligibleUnits(units,city).map(u=>u.id),['land','empty','laden']);
 assert.deepEqual(menu.marchEligibleUnits(units,city,'empty').map(u=>u.id),['empty']);
 assert.deepEqual(menu.marchUnitOptions(city,'naval',{stance:'storm'}),{});
 const inland={...city,coastal:false};
 assert.deepEqual(menu.marchEligibleUnits(units,inland).map(u=>u.id),['land']);
 assert.deepEqual(menu.marchEligibleUnits(units,{...inland,has_harbour:true},'empty').map(u=>u.id),['empty']);
 assert.deepEqual(menu.marchEligibleUnits(units,{...inland,coastal:undefined},'empty'),[]);
 assert.deepEqual(menu.marchEligibleUnits(units,{...city,own:false,allied:true},'empty'),[],'an allied city is not an own docking order');
 assert.deepEqual(menu.marchEligibleUnits(units,{...city,own:false},'laden').map(u=>u.id),['laden']);
 assert.deepEqual(menu.marchUnitOptions({...city,own:false},'naval',{stance:'storm'}),{});
});

test('map city target forwards actual ownership and port metadata instead of equating allied with own',async()=>{
 const {destFromHex}=await import('../render/map.js');
 const own=destFromHex({q:4,r:2},{terrain:'plains',coastal:true},{name:'Petras',own:true});
 assert.equal(own.own,true);assert.equal(own.coastal,true);assert.equal(menu.ownPortDestination(own),true);
 const allied=destFromHex({q:4,r:2},{terrain:'plains',coastal:true},{name:'Foreign',own:false,allied:true});
 assert.equal(allied.allied,true);assert.equal(allied.own,false);assert.equal(menu.ownPortDestination(allied),false);
});
