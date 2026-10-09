import test from 'node:test';
import assert from 'node:assert/strict';
class Element {
 constructor(){this.style={};this.classList={toggle(){}};this.events={};this.children=[];this.dataset={};this.clientWidth=1280;this.clientHeight=900;}
 addEventListener(k,fn){this.events[k]=fn;}
 append(...e){this.children.push(...e);}
 appendChild(e){this.append(e);}
 replaceChildren(){this.children=[];}
 querySelectorAll(){return [];}
 querySelector(){return this.children.find(e=>e.dataset.inlineResult);}
 setAttribute(){}
 getBoundingClientRect(){return {left:0,top:0};}
 getContext(){return {};}
}
const elements=new Map();const element=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};
globalThis.document={getElementById:element,createElement:()=>new Element(),addEventListener(){},querySelectorAll:()=>[]};
let renderedForecast;
globalThis.window={addEventListener(){},renderColonizePreviewHTML:p=>{renderedForecast=p;return '<p>Full forecast and hex list</p>';}};
globalThis.localStorage={getItem:()=>null};
globalThis.setInterval=()=>0;
globalThis.requestAnimationFrame=()=>0;
let army={Spearman:2},refused=false,delay=null,calls=[];
let forecast={grain:{est_net_per_tick:-0.1},goods:{timber:0.01,stone:0}},forecastDelay;
let fp={active:true,population:1000,spearmen_in_field:2,grain:{ticks_left:2.5},silver:{ticks_left:240},tick_seconds:6};
globalThis.fetch=async(url)=>{
 calls.push(url);
 if(url.endsWith('/founding/status'))return new Response(JSON.stringify(fp));
 if(url.includes('/colonize-preview')){if(forecastDelay)return new Promise(resolve=>{forecastDelay.resolve=resolve;});return new Response(JSON.stringify(forecast));}
 if(url.endsWith('/army')){
  if(delay)return new Promise(resolve=>{delay.resolve=resolve;});
  return new Response(JSON.stringify(army),{status:refused?403:200});
 }
 return new Promise(()=>{});
};
const {State}=await import('../state.js');
const {initMap}=await import('./map.js');initMap();
const marker={id:'foreign-province',settlement_id:'foreign-settlement',name:'Kyme',owner:'Other Wanax',culture:'ionian',walls:0,q:0,r:0};
async function click({terrain='plains',city=marker,host=false}={}){
 State.WORLD_ID='world';State.MY_SETTLEMENT_ID='own-settlement';State.tileData=[{q:0,r:0,terrain}];State.provinceData=city?[city]:[];State.unitsData=host?[{type:'nomadic_host',q:0,r:0}]:[];State.foreignUnitData=[];State.ruralData=[];State.founderPhase=host?fp:null;State.camera={x:0,y:0,zoom:1};
 const canvas=element('hex-canvas');canvas.events.pointerdown({pointerId:1,pointerType:'mouse',button:0,clientX:0,clientY:0});canvas.events.pointerup({pointerId:1,pointerType:'mouse',clientX:0,clientY:0});
 await new Promise(resolve=>setImmediate(resolve));
}

test('Q actual Host: only site assessment is primary and all existing detail stays in one closed Details',async()=>{
 await click({city:null,host:true});const html=element('ip-body-extra').innerHTML;
 assert.match(html,/<details id="ip-host-details"/,'Host keeps all secondary information in one Details');
 assert.equal((html.match(/<details/g)||[]).length,1);assert.doesNotMatch(html,/<details[^>]*\bopen\b/,'Details must begin closed');
 const [primary,details]=html.split('<details');
 assert.match(element('ip-found-summary').innerHTML,/Feeds itself:<\/span><span> no</,'negative net must not promise self-sufficiency');
 assert.match(element('ip-found-summary').innerHTML,/Timber:<\/span><span> yes</,'positive timber potential gives Timber yes');
 assert.match(element('ip-found-summary').innerHTML,/Stone:<\/span><span> no</,'zero stone gives Stone no');
 assert.doesNotMatch(primary,/Food lasts|Escort pay|people|Spearmen|Produces|Deposits|Full forecast/,'only summary belongs before Details');
 for(const text of ['people','Food lasts','Escort pay lasts','Spearmen','Messengers free','Produces','Deposits'])assert.ok(details.includes(text),text+' detail must remain');
 assert.deepEqual(renderedForecast,forecast,'same full forecast renderer gets same payload');
 assert.equal(element('ip-deposits-row').style.display,'none');assert.equal(element('ip-produces-row').style.display,'none','Host hides primary Produces row');
 assert.match(element('ip-foot').innerHTML,/Found the metropolis here/);
 assert.match(calls.find(u=>u.includes('/colonize-preview')),/q=0&r=0&pop=1000&seed=0&starter_farm=1/,'same forecast request');
});
test('Q actual Host: unrounded net and raw goods, not carried stocks or a new food formula, decide yes/no',async()=>{
 forecast={grain:{est_net_per_tick:0},goods:{timber:0,stone:0.001}};
 fp={...fp,grain:{amount:999,ticks_left:2.5},silver:{amount:999,ticks_left:240}};
 await click({city:null,host:true});let html=element('ip-found-summary').innerHTML;
 assert.match(html,/Feeds itself:<\/span><span> yes</,'zero net matches existing self-sufficiency boundary');assert.match(html,/Timber:<\/span><span> no</);assert.match(html,/Stone:<\/span><span> yes</,'small positive stone must not round to no');
 forecast={grain:{est_net_per_tick:-0.001},goods:{fish:10000}};await click({city:null,host:true});html=element('ip-found-summary').innerHTML;
 assert.match(html,/Feeds itself:<\/span><span> no</,'negative raw net stays no regardless of carried food or fish potential');assert.match(html,/Timber:<\/span><span> no</);assert.match(html,/Stone:<\/span><span> no</);
 forecast={};await click({city:null,host:true});html=element('ip-found-summary').innerHTML;
 assert.equal((html.match(/unknown/g)||[]).length,3,'missing forecast values must remain unknown');assert.doesNotMatch(html,/>yes</);
});
test('Q actual Host: produces restores on another hex and late forecast cannot overwrite that panel',async()=>{
 forecastDelay={};await click({city:null,host:true});const pending=forecastDelay;forecastDelay=null;
 await click({terrain:'fog',city:null});assert.equal(element('ip-produces-row').style.display,'','ordinary terrain keeps Produces');
 element('ip-found-preview').innerHTML='Other panel forecast';
 const before=element('ip-found-preview').innerHTML;
 pending.resolve(new Response(JSON.stringify({grain:{est_net_per_tick:55},goods:{timber:100}})));
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(element('ip-found-preview').innerHTML,before,'late Host forecast must not update another panel');
});

test('Q actual Host: archived real colonize-preview with timber 8 must show Timber yes',async()=>{
 const {readFileSync}=await import('node:fs');
 const real=JSON.parse(readFileSync(new URL('../../../../../docs/reviews/host-enkel/fixtures/real-colonize-preview.json',import.meta.url),'utf8'));
 assert.equal(real.forecast.goods.timber,8,'fixture preserves the real API commodity key');
 assert.equal(Object.hasOwn(real.forecast.goods,'lumber'),false,'real endpoint never renamed timber to lumber');
 forecast=real.forecast;await click({city:null,host:true});
 assert.match(element('ip-found-summary').innerHTML,/Timber:<\/span><span> yes</,'real forecast timber must show Timber yes');
});
