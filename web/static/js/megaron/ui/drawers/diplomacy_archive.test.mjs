import test from 'node:test';
import assert from 'node:assert/strict';
import {caseListHTML,caseDetailHTML,loadDipArchive} from './diplomacy_archive.js';
import {State} from '../../state.js';
const item={id:'case-id',subject:'Copper <terms>',counterpart_name:'<Wanax>',city_name:'Old Kyme',preview:'<hello>',latest_at:'2026-10-24T06:00:00Z',letter_count:2};
test('personal cases retain stable IDs and historical names and escape prose',()=>{
 const html=caseListHTML({items:[item,{...item,id:'another-case',subject:'Second matter'}]});
 assert.match(html,/data-case-open="case-id"/);assert.match(html,/data-case-open="another-case"/);assert.match(html,/Old Kyme/);assert.match(html,/&lt;Wanax&gt;/);assert.match(html,/Copper &lt;terms&gt;/);assert.doesNotMatch(html,/<hello>|online|read_at/);
 const detail=caseDetailHTML({subject:item.subject,items:[{id:'letter',sender_name:'Sender',recipient_name:'Recipient',origin_name:'Former city',destination_name:'Old port',message_text:'<secret>',sent_at:item.latest_at,reply_text:'<reply>'}]});
 assert.match(detail,/Former city/);assert.match(detail,/&lt;secret&gt;/);assert.match(detail,/Reply: &lt;reply&gt;/);assert.match(detail,/<details open>/);assert.doesNotMatch(detail,/data-queue-action/);
});
function node(){return {innerHTML:'',dataset:{},listeners:{},querySelector(){return null},querySelectorAll(){return []},addEventListener(e,f){this.listeners[e]=f}}}
async function rig(f){const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};const root=node(),controls=new Map();root.querySelector=s=>{if(s==='[data-case-private]'||s==='[data-case-memory]')return null;if(!controls.has(s))controls.set(s,node());return controls.get(s)};globalThis.document={getElementById:id=>id==='dtab-archive'?root:null};globalThis.localStorage={getItem:()=>null};State.WORLD_ID='w';try{await f(root,controls)}finally{Object.assign(globalThis,saved);State.WORLD_ID=null}}
test('actual archive encodes cursors and search and never reads letters',()=>rig(async(root,controls)=>{
 const calls=[];globalThis.fetch=async(url,opts)=>{calls.push({url,opts});return new Response(JSON.stringify({items:[item],next_cursor:'opaque+/='}))};
 await loadDipArchive(root);assert.match(calls[0].url,/correspondence\/cases\?limit=30$/);assert.match(root.innerHTML,/Personal archive/);
 await controls.get('[data-archive-next]').listeners.click();assert.match(calls[1].url,/cursor=opaque%2B%2F%3D/);
 await controls.get('[data-archive-search]').listeners.submit({preventDefault(){},currentTarget:{elements:{q:{value:'Old Kyme & copper'}}}});assert.match(calls[2].url,/q=Old\+Kyme\+%26\+copper/);assert.doesNotMatch(calls[2].url,/cursor=/);assert.ok(calls.every(c=>!c.opts.method));
}));
test('archive ignores a late response after world change',()=>rig(async(root)=>{
 let release;globalThis.fetch=()=>new Promise(resolve=>{release=resolve});const pending=loadDipArchive(root);State.WORLD_ID='elsewhere';root.innerHTML='new world';release(new Response(JSON.stringify({items:[item]})));await pending;assert.equal(root.innerHTML,'new world');
}));

test('case continuation uses exact case and only current contacted historical cities',()=>rig(async(root,controls)=>{
 const original={provinceData:State.provinceData,MY_SETTLEMENT_ID:State.MY_SETTLEMENT_ID};
 Object.assign(State,{MY_SETTLEMENT_ID:'own-city',provinceData:[{settlement_id:'dest',name:'Current port',own:false},{settlement_id:'rumour',own:false},{settlement_id:'unrelated',name:'Other',own:false}]});
 try {
  const open=node();open.dataset.caseOpen='case-id';root.querySelectorAll=s=>s==='[data-case-open]'?[open]:[];
  const posts=[];
  globalThis.fetch=async(url,opts={})=>{if(opts.method){posts.push({url,body:JSON.parse(opts.body)});return new Response('{}')};return new Response(JSON.stringify(url.includes('/case-id?')?{id:'case-id',subject:'Copper',items:[{id:'letter',sender_name:'Other',recipient_name:'Me',origin_name:'Historic port',destination_name:'Own',origin_id:'dest',destination_id:'own-city',sent_at:item.latest_at,message_text:'Hello'}]}:{items:[item]}))};
  await loadDipArchive(root);await open.listeners.click();assert.match(root.innerHTML,/value="dest"/);assert.doesNotMatch(root.innerHTML,/value="unrelated"|value="rumour"/);
  const status=node(),button=node(),form={elements:{message:{value:'Exact matter'},destination:{value:'dest'}},querySelector:s=>s==='button'?button:status};
  await controls.get('[data-case-compose]').listeners.submit({preventDefault(){},currentTarget:form});
  assert.deepEqual(posts,[{url:'/api/v1/worlds/w/settlements/own-city/messengers',body:{destination_id:'dest',message:'Exact matter',case_id:'case-id'}}]);assert.equal(form.elements.message.value,'');assert.match(status.textContent,/dispatched in this case/);
  State.provinceData=[];form.elements.message.value='Retain my draft';await controls.get('[data-case-compose]').listeners.submit({preventDefault(){},currentTarget:form});assert.equal(posts.length,1);assert.equal(form.elements.message.value,'Retain my draft');assert.match(status.textContent,/no longer/);
 } finally {Object.assign(State,original)}
}));
