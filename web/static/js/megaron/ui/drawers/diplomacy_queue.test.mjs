import test from 'node:test';
import assert from 'node:assert/strict';
import { queuePageHTML, queueActions, loadDipQueue } from './diplomacy_queue.js';
import { State } from '../../state.js';

const offer = { id:'offer', from_name:'<Kyme>', to_name:'Nostos', message:'<A letter>', queue_group:'waiting_you', offer_state:'pending', expires_at:'2999-01-01T00:00:00Z', missing_payment:'Need 5 silver', actions:['read','trade_decline'], trade_offer:{kind:'sell',offer_good:'copper',offer_qty:2,want_silver:5} };

test('queue shows missed and unaffordable offers, escapes text, and uses only server response actions',()=>{
 const html=queuePageHTML({items:[offer,{...offer,id:'old',queue_group:'completed',offer_state:'expired',actions:['read']}]});
 assert.match(html,/Waiting on you/);assert.match(html,/Past correspondence/);assert.match(html,/Expired without answer/);assert.match(html,/Need 5 silver/);assert.match(html,/&lt;Kyme&gt;/);assert.match(html,/&lt;A letter&gt;/);assert.match(html,/Buy 2 copper/);assert.match(html,/Answer deadline/);
 assert.equal((html.match(/data-queue-action="trade_decline"/g)||[]).length,1);assert.doesNotMatch(html,/data-queue-action="trade_accept"/);
 assert.match(html,/Groups apply to this page/);assert.match(html,/<p class="dip-queue-preview">&lt;A letter&gt;<\/p>/);
});

test('a page held across expiry loses accept and decline without inventing server mutations',()=>{
 const stale={...offer,expires_at:'2000-01-01T00:00:00Z',actions:['read','trade_accept','trade_decline']};
 assert.deepEqual(queueActions(stale),['read']);
 const html=queuePageHTML({items:[stale]});assert.match(html,/Expired without answer/);assert.doesNotMatch(html,/data-queue-action=/);assert.doesNotMatch(html,/You can still decline/);
 // A completed accepted transaction never becomes expired merely because its old deadline passed.
 assert.match(queuePageHTML({items:[{...stale,queue_group:'completed',offer_state:'accepted',actions:['read']}]}),/>Accepted</);
});

function node(){return {innerHTML:'',dataset:{},listeners:{},querySelector(){return null;},querySelectorAll(){return [];},addEventListener(event,callback){this.listeners[event]=callback;}};}
async function rig(run){
 const saved={document:globalThis.document,fetch:globalThis.fetch,localStorage:globalThis.localStorage};const root=node();const controls=new Map();const calls=[];
 root.querySelector=s=>{if(!controls.has(s))controls.set(s,node());return controls.get(s)};
 globalThis.document={getElementById:id=>id==='dtab-queue'?root:null};globalThis.localStorage={getItem:()=>null};State.WORLD_ID='world';
 try{await run({root,controls,calls});}finally{Object.assign(globalThis,saved);State.WORLD_ID=null;}
}

test('actual loader pages by encoded cursor, refreshes from newest, and listing never posts read receipts',()=>rig(async({root,controls,calls})=>{
 globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});return new Response(JSON.stringify(url.includes('cursor=')?{items:[{...offer,id:'old',offer_state:'expired',queue_group:'completed',actions:['read']}]}:{items:[offer],next_cursor:'opaque+/='}))};
 await loadDipQueue(root);assert.match(root.innerHTML,/Need 5 silver/);assert.equal(calls.length,1);assert.ok(calls.every(c=>!c.opts.method));
 await controls.get('[data-queue-next]').listeners.click();assert.match(calls[1].url,/cursor=opaque%2B%2F%3D/);assert.match(root.innerHTML,/Expired without answer/);
 await controls.get('[data-queue-refresh]').listeners.click();assert.ok(!calls[2].url.includes('cursor='));assert.ok(calls.every(c=>!c.opts.method));
}));

test('actual loader ignores a late response after world change or drawer replacement',()=>rig(async({root})=>{
 let release;globalThis.fetch=()=>new Promise(resolve=>{release=resolve});const pending=loadDipQueue(root);
 State.WORLD_ID='new-world';root.innerHTML='new drawer';release(new Response(JSON.stringify({items:[offer]})));await pending;assert.equal(root.innerHTML,'new drawer');
}));

test('opening one actual letter acknowledges only that ID, and replying uses the same delivered messenger',()=>rig(async({root,calls})=>{
 const status=node(),note=node(),reply={value:'Carry this answer home'},actions=node();
 const article={dataset:{letter:'delivered-unknown-origin'},querySelector:s=>s==='[role="status"]'?status:s==='.dip-queue-note'?note:s==='[data-queue-reply]'?reply:s==='.dip-queue-actions'?actions:null,querySelectorAll:()=>[]};
 const details={...node(),dataset:{openLetter:'delivered-unknown-origin'},open:false,closest:()=>article};
 const button={...node(),dataset:{queueAction:'reply'},closest:()=>article};
 root.querySelectorAll=s=>s==='[data-open-letter]'?[details]:s==='[data-queue-action]'?[button]:[];
 globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});return new Response(JSON.stringify(opts.method?{read:true,returns_at:'2999-01-01T00:00:00Z'}:{items:[{...offer,id:'delivered-unknown-origin',from_id:'unknown-to-recipient',trade_offer:null,offer_state:'',actions:['read','reply']}]}))};
 await loadDipQueue(root);assert.equal(calls.length,1,'listing did not read');
 details.open=true;await details.listeners.toggle();assert.equal(calls[1].url,'/api/v1/worlds/world/messengers/delivered-unknown-origin/read');
 assert.match(note.textContent,/Read$/);assert.equal(calls.length,2);
 await button.listeners.click();assert.equal(calls[2].url,'/api/v1/worlds/world/messengers/delivered-unknown-origin/reply');assert.deepEqual(JSON.parse(calls[2].opts.body),{reply:'Carry this answer home'});assert.equal(status.textContent,'Reply dispatched.');
 assert.ok(calls.every(c=>!c.url.includes('/settlements/')),'return post does not create a new normal-send route');
}));

test('a second queue mount supersedes an older fetch in the same drawer and world',()=>rig(async({root})=>{
 let release;globalThis.fetch=()=>new Promise(resolve=>{release=resolve});const older=loadDipQueue(root);
 globalThis.fetch=async()=>new Response(JSON.stringify({items:[{...offer,from_name:'Newer response'}]}));await loadDipQueue(root);assert.match(root.innerHTML,/Newer response/);
 release(new Response(JSON.stringify({items:[{...offer,from_name:'Stale response'}]})));await older;assert.match(root.innerHTML,/Newer response/);assert.doesNotMatch(root.innerHTML,/Stale response/);
}));

test('return-post actions obey server closure and display saved tick deadline without guessing from an elapsed ISO',()=>{
 const original={CURRENT_TICK:State.CURRENT_TICK,TICK_SECONDS:State.TICK_SECONDS,TICK_ANCHOR_MS:State.TICK_ANCHOR_MS};
 Object.assign(State,{CURRENT_TICK:100,TICK_SECONDS:3600,TICK_ANCHOR_MS:Date.now()});
 try{
  const letter={...offer,trade_offer:null,offer_state:'',actions:['read','reply'],reply_before:'2000-01-01T00:00:00Z',reply_deadline_tick:102,reply_window_closed:false};
  const open=queuePageHTML({items:[letter]});assert.match(open,/Return post before/);assert.doesNotMatch(open,/2000/);assert.match(open,/data-queue-action="reply"/);
  assert.deepEqual(queueActions({...letter,reply_window_closed:true}),['read']);
  const closed=queuePageHTML({items:[{...letter,queue_group:'completed',reply_window_closed:true}]});assert.match(closed,/Return post closed/);assert.doesNotMatch(closed,/data-queue-action="reply"/);
  const answered=queuePageHTML({items:[{...letter,queue_group:'completed',reply_window_closed:true,reply_text:'I already answered',actions:['read']}]});assert.match(answered,/Reply dispatched/);assert.doesNotMatch(answered,/Return post closed/);
  const legacy=queuePageHTML({items:[{...letter,reply_before:null,reply_deadline_tick:null}]});assert.match(legacy,/Return post deadline unavailable/);assert.match(legacy,/data-queue-action="reply"/);
 }finally{Object.assign(State,original);}
});
