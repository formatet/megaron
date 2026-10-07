import test from 'node:test';
import assert from 'node:assert/strict';
import { recallBatchBody, recallBatchReportHTML, sendRecallBatch, recallBatchControlsHTML } from './recall_batch.js';
test('selection and filters match CLI and reject malformed coordinates', () => {
 assert.deepEqual(recallBatchBody('5,-3','explore'),{all:true,target_q:5,target_r:-3,intent:'explore'});
 assert.deepEqual(recallBatchBody(),{all:true});
 for(const x of ['1','1,2,3','1.5,0','NaN,0']) assert.throws(()=>recallBatchBody(x));
 assert.match(recallBatchControlsHTML(),/onclick="unitRecallAll\(\)"/);
});
test('groups, singles and every named refusal are visible and escaped',()=>{
 const html=recallBatchReportHTML({groups:[{unit_ids:['a','b'],q:2,r:0,courier_due_tick:9},{unit_ids:['c'],q:4,r:1,passage_status:'awaiting_passage'}],results:[{name:'Alpha',status:'order_dispatched'},{name:'Host',status:'order_applied'},{name:'<Delta>',status:'rejected',reason:'not marching <x>'}]});
 for(const text of ['Group:','Single:','game day 9','awaiting_passage','Alpha: recall sent','Host: recall given in person','&lt;Delta&gt;: rejected','not marching &lt;x&gt;']) assert.ok(html.includes(text),text);
 assert.ok(!html.includes('<Delta>'));assert.match(recallBatchReportHTML({results:[]}),/No units match/);
});
test('batch action uses authenticated fetch bridge and returns server report',async()=>{
 const report={groups:[],results:[{name:'Delta',status:'rejected',reason:'not marching'}]};
 const fetcher=async(url,opts)=>{assert.equal(url,'/api/v1/worlds/w/units/recall');assert.equal(opts.method,'POST');assert.deepEqual(JSON.parse(opts.body),{all:true,intent:'explore'});return {ok:true,json:async()=>report}};
 assert.equal(await sendRecallBatch(fetcher,'w',recallBatchBody('','explore')),report);
 await assert.rejects(sendRecallBatch(async()=>({ok:false,json:async()=>({error:'not authenticated'})}),'w',{all:true}),/not authenticated/);
});
