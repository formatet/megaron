import test from 'node:test';
import assert from 'node:assert/strict';
import {State} from '../../state.js';
import {loadDiplomacyDrawer} from './diplomacy.js';

// Exercise the actual drawer, real fetchAuth and actual row renderer.
// Fake only the transport and minimal DOM; no copied endpoint selector.
test('Correspondence reads host and city outboxes and renders their letters', async () => {
  const original={fetch:globalThis.fetch,document:globalThis.document,localStorage:globalThis.localStorage};
  const body={innerHTML:'',querySelectorAll:()=>[]};
  const threads={innerHTML:'',querySelectorAll:()=>[]};
  globalThis.document={getElementById:id=>id==='diplomacy-body'?body:id==='dtab-threads'?threads:null};
  globalThis.localStorage={getItem:()=>null};
  State.WORLD_ID='test-world';State.provinceData=[];
  try {
    for (const settlement of [null,'my-city']) {
      State.MY_SETTLEMENT_ID=settlement;
      const calls=[];
      const endpoint=settlement?'/settlements/my-city/messengers':'/founding/messengers';
      globalThis.fetch=async url=>{
        calls.push(url);
        const data=url.endsWith(endpoint)?[{id:'runner',destination_name:'Nostos',destination_id:'recipient',message_text:'From my host <hello>',status:'outbound',sent_at:new Date().toISOString(),arrives_at:new Date(Date.now()+3600000).toISOString()}]:[];
        return new Response(JSON.stringify(data),{status:200});
      };
      await loadDiplomacyDrawer();
      assert.ok(calls.some(url=>url.endsWith(endpoint)),`actual drawer must fetch ${endpoint}`);
      assert.match(threads.innerHTML,/Nostos/);
      assert.match(threads.innerHTML,/From my host &lt;hello&gt;/);
      assert.match(threads.innerHTML,/en route/);
      assert.doesNotMatch(threads.innerHTML,/No correspondence yet/);
    }
  } finally {
    Object.assign(globalThis,original);
    State.MY_SETTLEMENT_ID=null;State.WORLD_ID=null;
  }
});
