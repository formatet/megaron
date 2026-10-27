import test from 'node:test';
import assert from 'node:assert/strict';
import { realmKultHTML,prayerActionID } from './kult_realm.js';

test('three temples and two gods in different cities retain separate city actions',()=>{
 const temples=[{id:'one',name:'Stillness',god_id:'stillness',city_name:'Tylissos',province_id:'p1',settlement_id:'s1'},{id:'two',name:'Asterion',god_id:'asterion',city_name:'Amnissos',province_id:'p2',settlement_id:'s2'},{id:'three',name:'Asterion',god_id:'asterion',city_name:'Kommos',province_id:'p3',settlement_id:'s3'}].map(t=>({...t,level:1,staffed:1,capacity:2,offer:{fed:true,offering:{grain:.25}}}));
 const prayers=temples.map(t=>({id:t.god_id,settlement_id:t.settlement_id,city_name:t.city_name,temple:t.name,name:'Prayer',min_kharis:5,has_temple:true}));
 const html=realmKultHTML({temples,prayers,unbuilt_temples:[{name:'Potnia',city_name:'Kommos',province_id:'p3'}]},'temples',p=>`action:${p.id}`);
 assert.equal((html.match(/data-cult-temple=/g)||[]).length,3);
 for(const t of temples){assert.ok(html.includes(t.city_name));assert.ok(html.includes(`data-cult-city="${t.province_id}"`));assert.ok(html.includes(`data-cult-demolish="${t.id}"`));}
 assert.ok(html.indexOf('data-cult-temple="three"')<html.indexOf('Not built'));
 assert.ok(html.includes('action:s2__asterion'));assert.ok(html.includes('action:s3__asterion'));
 assert.ok(html.includes('data-cult-panel="temples">'));assert.ok(html.includes('data-cult-panel="prayers" hidden'));
 assert.notEqual(prayerActionID(prayers[1]),prayerActionID(prayers[2]));
});

test('realm labels are escaped and absent projections do not invent kharis',()=>{
 const html=realmKultHTML({temples:[{id:'x',name:'<temple>',city_name:'<city>',offer:{}}],prayers:[],unbuilt_temples:[]});
 assert.ok(html.includes('&lt;temple&gt;'));assert.ok(html.includes('&lt;city&gt;'));assert.ok(!html.includes('Kharis 0'));
});
