import test from 'node:test';
import assert from 'node:assert/strict';
class El {
 constructor(){this.children=[];this.dataset={};this.events={};this.style={};this.classList={toggle(){}};}
 append(...xs){this.children.push(...xs);}appendChild(x){this.append(x);}replaceChildren(){this.children=[];}
 querySelector(s){return s==='[data-inline-result]'?this.children.find(x=>x.dataset.inlineResult):null;}
 querySelectorAll(){return [];}setAttribute(){}addEventListener(k,fn){this.events[k]=fn;}
 set innerHTML(value){this.html=value;this.children=[];}get innerHTML(){return this.html || '';}
}
const els=new Map(['city-bld-sec','kult-body','dip-trade-m','dip-passage-m','war-abandon-res'].map(k=>[k,new El()]));
const noopEl=new Proxy({}, {get:(_t,k)=>k==='style'?{}:()=>noopEl,set:()=>true});
globalThis.ResizeObserver ??= class { observe() {} };
globalThis.document={getElementById:k=>k==='net-status'?null:els.get(k)||noopEl,querySelector:()=>null,querySelectorAll:()=>[],createElement:()=>new El(),addEventListener(){}};
globalThis.window={addEventListener(){},matchMedia:()=>({matches:false,addEventListener(){}})};
globalThis.localStorage={getItem:()=>null,setItem(){}};
globalThis.alert=globalThis.confirm=()=>assert.fail('browser dialog must never be called');
const {State}=await import('../../state.js');
const {cancelBuild}=await import('./city.js');
const {okRite}=await import('./kult.js');
const {dipCancel,dipArrangePassage,dipCallBack}=await import('./diplomacy.js');
const {warAbandon}=await import('./war.js');
State.WORLD_ID='w';State.MY_SETTLEMENT_ID='s';State.provinceData=[{id:'p',own:true,is_capital:true}];
const calls=[];
globalThis.fetch=async(url,opts={})=>{calls.push([url,opts]);return new Response(JSON.stringify({error:'Server refused <literally>'}),{status:422});};
test('K: cancellation and passage refusals keep buttons enabled and show server text beside the action',async()=>{
 calls.length=0;await cancelBuild('p','q');assert.equal(els.get('city-bld-sec').children.at(-1).textContent,'Server refused <literally>');
 const button=new El();button.parentElement=new El();await dipCancel('m',button);assert.equal(button.disabled,false);assert.equal(els.get('dip-trade-m').children.at(-1).textContent,'Server refused <literally>');
 els.set('ship-select',{value:'ship'});await dipArrangePassage('m','ship-select',button);assert.equal(button.disabled,false);assert.equal(els.get('dip-passage-m').children.at(-1).textContent,'Server refused <literally>');
 await dipCallBack('m',button);assert.equal(button.disabled,false);assert.equal(button.parentElement.children.at(-1).textContent,'Server refused <literally>');
 assert.deepEqual(calls.map(x=>x[1].method),['DELETE','POST','POST','POST']);assert.equal(calls[2][1].body,JSON.stringify({ship_id:'ship'}));
});
test('K: rite refusal stays in Kult; success survives drawer refresh',async()=>{
 await okRite('prayer');assert.equal(els.get('kult-body').children.at(-1).textContent,'Server refused <literally>');
 globalThis.fetch=async(url,opts={})=>{
  if(opts.method==='POST')return new Response(JSON.stringify({success:true,message:'The gods answered.'}),{status:200});
  if(url.endsWith('/goods')||url.endsWith('/actions'))return new Response('[]',{status:200});
  return new Response(JSON.stringify({settlement:{},divine_mood:'Indifferent'}),{status:200});
 };
 await okRite('prayer');assert.equal(els.get('kult-body').children.at(-1)?.textContent,'The gods answered.','success must survive refresh');
});
test('K: real abandon handler cannot POST before inline confirmation; refusal remains visible',async()=>{
 calls.length=0;globalThis.fetch=async(url,opts={})=>{calls.push([url,opts]);return new Response(JSON.stringify({error:'Cannot abandon capital'}),{status:422});};
 await warAbandon('s','Nostos');assert.equal(calls.length,0,'no pre-confirmation POST');
 const host=els.get('war-abandon-res');await host.children[1].events.click();assert.equal(calls.length,1);assert.equal(calls[0][1].method,'POST');assert.equal(host.children.at(-1).textContent,'Cannot abandon capital');
});
