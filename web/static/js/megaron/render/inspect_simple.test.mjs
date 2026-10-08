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
globalThis.window={addEventListener(){},renderColonizePreviewHTML:()=>'<p>Forecast</p>'};
globalThis.localStorage={getItem:()=>null};
globalThis.setInterval=()=>0;
globalThis.requestAnimationFrame=()=>0;
let army={Spearman:2},refused=false,delay=null,calls=[];
let fp={active:true,population:1000,spearmen_in_field:2,grain:{ticks_left:2.5},silver:{ticks_left:240},tick_seconds:6};
globalThis.fetch=async(url)=>{
 calls.push(url);
 if(url.endsWith('/founding/status'))return new Response(JSON.stringify(fp));
 if(url.includes('/colonize-preview'))return new Response('{}');
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
 const canvas=element('hex-canvas');canvas.events.mousedown({clientX:0,clientY:0});canvas.events.mouseup({clientX:0,clientY:0});
 await new Promise(resolve=>setImmediate(resolve));
}
test('L actual inspect: owner and qualitative defence replace Culture/Walls/DP, actions keep IDs',async()=>{
 await click();assert.equal(element('ip-defence').textContent,'strong','visible defenders become qualitative defence');
 assert.equal(element('ip-owner').textContent,'Other Wanax');assert.match(element('ip-foot').innerHTML,/sendMessengerFromInspect\('foreign-settlement'\)/);assert.match(element('ip-foot').innerHTML,/March here/);
 let dest;window.openMarchCtx=d=>dest=d;element('ip-march-btn').events.click({clientX:0,clientY:0});assert.equal(dest.q,0);assert.equal(dest.known,true);assert.equal(dest.isSettlement,true);
 army={};await click();assert.equal(element('ip-defence').textContent,'weak');
 await click({city:{...marker,walls:1,allied:true}});assert.equal(element('ip-defence').textContent,'strong');assert.equal(element('ip-owner').textContent,'Other Wanax (allied)');
 refused=true;await click();assert.equal(element('ip-defence').textContent,'unknown','refused data cannot imply weak defence');refused=false;
 const {readFileSync}=await import('node:fs');const html=readFileSync(new URL('../../../map.html',import.meta.url),'utf8');
 assert.doesNotMatch(html,/id="ip-(culture|walls|army)(-row)?"/,'obsolete inspect blocks removed');assert.match(html,/Defence/);
});
test('L actual inspect: fog never exposes marker/army and late army reply cannot update a different panel',async()=>{
 delay={};await click();const pending=delay;delay=null;
 const before=calls.filter(u=>u.endsWith('/army')).length;
 await click({terrain:'fog'});assert.equal(element('ip-owner-row').style.display,'none');assert.equal(element('ip-defence-row').style.display,'none','fog hides defence');assert.equal(element('ip-name').textContent,'Unexplored land');assert.equal(calls.filter(u=>u.endsWith('/army')).length,before,'fog never fetches army');
 element('ip-defence').textContent='unchanged';pending.resolve(new Response(JSON.stringify({Spearman:20})));await new Promise(resolve=>setImmediate(resolve));assert.equal(element('ip-defence').textContent,'unchanged','late army reply must not change fog panel');
 await click({city:null});assert.equal(element('ip-owner-row').style.display,'none');assert.equal(element('ip-defence-row').style.display,'none');assert.match(element('ip-foot').innerHTML,/Colonize/,'empty-ground action stays');
});
test('L actual Host: food and escort pay durations use exact game days in words, never wall time',async()=>{
 await click({city:null,host:true});let html=element('ip-body-extra').innerHTML;
 assert.match(html,/Food lasts two point five game days/,'host food uses exact unscaled game days');assert.match(html,/Escort pay lasts two hundred forty game days/);assert.doesNotMatch(html.replace(/<[^>]*>/g,''),/tick left|real time|NaN|\d/,'Host status uses player words');assert.match(element('ip-foot').innerHTML,/Found the metropolis here/,'founding stays');
 fp={...fp,grain:{ticks_left:null},silver:{ticks_left:1},spearmen_in_field:1};await click({city:null,host:true});html=element('ip-body-extra').innerHTML;assert.match(html,/Food lasts indefinitely/);assert.match(html,/Escort pay lasts one game day</);assert.match(html,/one Spearmen cohort in the field/);
});
