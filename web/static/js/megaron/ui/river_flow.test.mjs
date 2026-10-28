import test from 'node:test';
import assert from 'node:assert/strict';
import {riverFlowLines,drawRiverFlow} from './river_flow.js';
const river={q:0,r:0,terrain:'river',river_edges:[0,1],river_flow_known:true,river_upstream_edges:[1],river_downstream_edges:[0]};
test('river text uses only sanctioned directions and keeps fog opaque',()=>{
 assert.deepEqual(riverFlowLines(river,[{q:1,r:0,terrain:'fog',secret_name:'DO NOT SHOW'}]),['Flows from north-west toward south-east.','Continues into the fog toward south-east and north-west.']);
 assert.deepEqual(riverFlowLines({...river,terrain:'fog'}),[]);
 assert.deepEqual(riverFlowLines({...river,river_flow_known:false,river_edges:[]}),['Flow direction unavailable.','The river ends here.']);
});
test('flow glyph uses whole pixels only on a known river with proven direction',()=>{
 const pixels=[];const ctx={save(){},restore(){},fillRect(...p){pixels.push(p);}};
 drawRiverFlow(ctx,20.3,20.7,river,'water');assert.ok(pixels.length>0);assert.ok(pixels.every(p=>p.every(Number.isInteger)&&p[2]===1&&p[3]===1));
 pixels.length=0;drawRiverFlow(ctx,0,0,{...river,terrain:'fog'},'water');drawRiverFlow(ctx,0,0,{...river,river_flow_known:false},'water');assert.equal(pixels.length,0);
});


test('a known river exposes its connection kind through fog, with no neighbour details',()=>{
 const lines=riverFlowLines({...river,river_connections:[{edge:0,terrain:'river_ford',name:'SECRET',units:[{name:'SECRET'}]},{edge:1,terrain:'coastal_sea'}]},[{q:1,r:0,terrain:'fog'}]);
 assert.ok(lines.includes('Connections: south-east — ford; north-west — coastal water.'));
 assert.ok(!lines.join(' ').includes('SECRET'));
});
