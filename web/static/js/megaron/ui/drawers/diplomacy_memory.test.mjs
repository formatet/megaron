import test from 'node:test';
import assert from 'node:assert/strict';
import {memoryHTML,citationFormHTML,mountCaseMemory} from './diplomacy_memory.js';
const page={transactions:[{id:'trade',case_id:'source-case',subject:'Copper',sender_name:'<Sender>',recipient_name:'Recipient',origin_name:'Kyme',destination_name:'Nostos',terms:{kind:'sell',offer_qty:2,offer_good:'copper',want_silver:5}}],citations:[{id:'cite',case_id:'source-case',source:'message',quote:'<An untrue promise>'}],next_cursor:'opaque+/='};
test('memory separates accepted engine terms and escaped personal quotes without inventing fulfillment',()=>{
 const html=memoryHTML(page);for(const text of ['Accepted agreements','2 copper for 5 silver','My notes from letters','My note','&lt;An untrue promise&gt;','Delivery outcome unavailable','data-memory-case="source-case"'])assert.ok(html.includes(text),text);assert.match(html,/<p class="dip-queue-note">My note · letter<\/p>/);assert.doesNotMatch(html,/<An untrue promise>|Betrayed|Debt/);
 const form=citationFormHTML({id:'letter',message_text:'<Letter>',reply_text:'A reply'});assert.match(form,/data-citation-letter="letter"/);assert.match(form,/&lt;Letter&gt;/);assert.match(form,/value="reply"/);
});
function node(){return {innerHTML:'',textContent:'',dataset:{},disabled:false,listeners:{},addEventListener(e,f){this.listeners[e]=f}}}
async function rig(fn){const saved={fetch:globalThis.fetch,localStorage:globalThis.localStorage,document:globalThis.document};globalThis.localStorage={getItem:()=>null};globalThis.document={getElementById:()=>null};const root=node(),controls=new Map();root.querySelector=s=>{if(!controls.has(s))controls.set(s,node());return controls.get(s)};root.querySelectorAll=()=>[];const archive={querySelectorAll:()=>[]};let active=true;const calls=[];globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});return new Response(JSON.stringify(page))};try{await fn({root,controls,archive,calls,alive:()=>active,close:()=>{active=false}})}finally{Object.assign(globalThis,saved)}}
test('memory loader uses same case and stable cursor and ignores late case response',()=>rig(async({root,controls,archive,calls,alive,close})=>{
 await mountCaseMemory(root,'w','case',alive,archive,[],()=>{});assert.equal(calls[0].url,'/api/v1/worlds/w/correspondence/cases/case/memory?limit=30');await controls.get('[data-memory-next]').listeners.click();assert.match(calls[1].url,/cursor=opaque%2B%2F%3D/);
 let release;globalThis.fetch=()=>new Promise(resolve=>{release=resolve});const pending=controls.get('[data-memory-refresh]').listeners.click();close();root.innerHTML='new case';release(new Response(JSON.stringify(page)));await pending;assert.equal(root.innerHTML,'new case');
}));
test('keeping a private passage sends only selected exact source and failure retains local quote',()=>rig(async({root,controls,archive,calls,alive})=>{
 const status=node(),button=node(),source={...node(),value:'reply'},quote={value:'Exact reply'},form={...node(),dataset:{citationLetter:'letter'},elements:{source,quote},querySelector:s=>s==='button'?button:status};archive.querySelectorAll=()=>[form];
 await mountCaseMemory(root,'w','case',alive,archive,[{id:'letter',message_text:'Actual letter',reply_text:'Exact reply'}],()=>{});
 globalThis.fetch=async(url,opts={})=>{calls.push({url,opts});return new Response(JSON.stringify({error:'not an exact passage'}),{status:400})};
 await form.listeners.submit({preventDefault(){}});assert.deepEqual(JSON.parse(calls.at(-1).opts.body),{letter_id:'letter',source:'reply',quote:'Exact reply'});assert.equal(quote.value,'Exact reply');assert.match(status.textContent,/not an exact passage/);
}));

test('memory source link carries exact stable letter identity across cases',()=>rig(async({root,controls,archive,alive})=>{
 const button={...node(),dataset:{memoryCase:'old-case',memoryLetter:'old-letter'}};root.querySelectorAll=s=>s==='[data-memory-case]'?[button]:[];let selected;
 await mountCaseMemory(root,'w','current-case',alive,archive,[],(...args)=>{selected=args});button.listeners.click();assert.deepEqual(selected,['old-case','old-letter']);
}));
