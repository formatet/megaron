import test from 'node:test';
import assert from 'node:assert/strict';
class Element {
 constructor(){this.style={};this.classList={toggle(){}};this.events={};this.children=[];this.dataset={};this.clientWidth=1280;this.clientHeight=900;}
 addEventListener(k,fn){this.events[k]=fn;}
 append(...e){this.children.push(...e);}
 appendChild(e){this.append(e);}
 replaceChildren(){this.children=[];}
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
let dialogs=0,reloads=0,calls=[],reply,resolveRequest;
globalThis.confirm=()=>{dialogs++;return false;};
globalThis.location={reload(){reloads++;}};
const fp={active:true,population:1000,grain:{amount:1200},tick_seconds:6};
globalThis.fetch=async(url,options={})=>{
 if(url.endsWith('/founding/status'))return new Response(JSON.stringify(fp));
 if(url.includes('/colonize-preview'))return new Response('{}');
 if(url.endsWith('/founding/settle')){calls.push({url,options});return new Promise(resolve=>{resolveRequest=()=>resolve(reply);});}
 // Leave the initial map load pending. The test sets the same map input explicitly.
 return new Promise(()=>{});
};
const {State}=await import('../state.js');
const {initMap}=await import('./map.js');initMap();
async function openHost(){
 State.WORLD_ID='world';State.tileData=[{q:0,r:0,terrain:'plains'}];State.provinceData=[];State.unitsData=[{type:'nomadic_host',q:0,r:0}];State.founderPhase=fp;State.camera={x:0,y:0,zoom:1};
 const canvas=element('hex-canvas');canvas.events.pointerdown({pointerId:1,pointerType:'mouse',button:0,clientX:0,clientY:0});canvas.events.pointerup({pointerId:1,pointerType:'mouse',clientX:0,clientY:0});
 await new Promise(resolve=>setImmediate(resolve));
 element('ip-settle-err').replaceChildren();
}
test('Founding actual map click: opening/cancel never sends; confirmed request is single and retains body/reload',async()=>{
 await openHost();const button=element('ip-settle-btn');await button.events.click();
 assert.equal(dialogs,0,'founding must confirm inline without browser dialog');
 const host=element('ip-settle-err');assert.match(host.children[0].textContent,/host dissolves — forever/);
 assert.equal(host.children[1].textContent,'Found the metropolis');assert.equal(host.children[2].textContent,'Keep travelling');
 assert.equal(calls.length,0,'opening must not found');host.children[2].events.click();assert.equal(calls.length,0,'cancel must not found');assert.equal(host.children.length,0);
 await button.events.click();const yes=host.children[1];const sent=yes.events.click();yes.events.click();await button.events.click();
 assert.equal(host.children[1],yes,'pending founding cannot reopen confirmation');
 assert.equal(calls.length,1,'one founding request while pending');assert.equal(button.disabled,true);
 assert.equal(calls[0].url,'/api/v1/worlds/world/founding/settle');assert.equal(calls[0].options.method,'POST');assert.deepEqual(JSON.parse(calls[0].options.body),{});
 reply=new Response('{}');resolveRequest();await sent;assert.equal(reloads,1,'success refreshes changed world');
});
test('Founding actual handler: server and network errors stay inline and allow retry',async()=>{
 await openHost();const button=element('ip-settle-btn');button.disabled=false;
 await button.events.click();let sent=element('ip-settle-err').children[1].events.click();reply=new Response(JSON.stringify({error:'<move further>'}),{status:422});resolveRequest();await sent;
 assert.equal(reloads,1);assert.equal(button.disabled,false,'failed founding can retry');assert.equal(element('ip-settle-err').querySelector().textContent,'<move further>','server error stays literal inline');
 const previous=globalThis.fetch;globalThis.fetch=async()=>{throw new Error('offline');};
 try{await button.events.click();await element('ip-settle-err').children[1].events.click();assert.equal(button.disabled,false);assert.match(element('ip-settle-err').querySelector().textContent,/Could not/);}finally{globalThis.fetch=previous;}
});
