import test from 'node:test';import assert from 'node:assert/strict';
import {remainingRoute,routePosition,liveRouteSegments,foreignArrow,actorRoute,routeProgress,drawMovement,ROUTE_PATTERNS} from './routes.js';
const px=(q,r)=>({x:q*10,y:r*10}),path=[[0,0],[1,0],[1,1],[2,1]],live=()=>true;
test('remaining route starts at the actor, retains its actual corners, and shrinks to nothing at arrival',()=>{
 assert.deepEqual(remainingRoute(path,0,px).map(p=>[p.x,p.y]),[[0,0],[10,0],[10,10],[20,10]]);
 const middle=remainingRoute(path,.5,px);assert.deepEqual(middle.map(p=>[p.x,p.y]),[[10,5],[10,10],[20,10]]);assert.deepEqual(middle[0],routePosition(path,.5,px));
 assert.deepEqual(remainingRoute(path,1,px),[]);assert.deepEqual(remainingRoute(null,.5,px),[]);
});
test('remaining route and actor share weighted saved-step timing and the world tick phase',()=>{
 assert.deepEqual(remainingRoute(path,.5,px,[1,8,1]).map(p=>[p.x,p.y]),[[10,10],[20,10]]);
 assert.equal(routeProgress({departure_tick:10,arrival_tick:12},0,10.5),.25);
 assert.equal(routePosition(path,.1,px,[1,8,1]).index,2);
 assert.equal(routePosition(path,.9,px,[1,8,1]).index,3);
});
test('remaining route never paints across remembered or unknown hexes',()=>{
 const points=remainingRoute(path,0,px);
 assert.deepEqual(liveRouteSegments(points,(q,r)=>r===1),[[points[2],points[3]]]);
 assert.deepEqual(liveRouteSegments(points,()=>false),[]);
});
test('foreign arrow reads only current live position and compass, never private route or destination',()=>{
 const actor={q:1,r:1,heading:'east'};
 for(const key of ['path','target_q','target_r','dest_q','dest_r','arrives_at','departs_at'])Object.defineProperty(actor,key,{get(){throw Error('foreign route leaked into drawing');}});
 assert.deepEqual(foreignArrow(actor,px,live),{type:'arrow',kind:'army',x:10,y:10,heading:'east'});
 assert.equal(foreignArrow(actor,px,()=>false),null);
 assert.equal(foreignArrow({q:1,r:1},px,live),null);
});
test('four own actor kinds have distinct ink patterns and use the same remaining-route function',()=>{
 assert.equal(new Set(Object.values(ROUTE_PATTERNS).map(JSON.stringify)).size,4);
 for(const kind of Object.keys(ROUTE_PATTERNS)){
  const primitive=actorRoute({path,departs_at:'2026-10-12T00:00:00Z',arrives_at:'2026-10-12T00:00:10Z'},kind,Date.parse('2026-10-12T00:00:05Z'),null,px,live);
  assert.equal(primitive.kind,kind);assert.deepEqual(primitive.segments[0][0],routePosition(path,.5,px));
 }
});
test('movement ink uses integer pixels, with no stroke, blur, or background plate',()=>{
 const calls=[];const ctx={save(){},restore(){},fillRect(x,y,w,h){calls.push([x,y,w,h,this.fillStyle]);}};
 drawMovement(ctx,actorRoute({path,departure_tick:0,arrival_tick:1},'ship',0,.5,px,live));
 drawMovement(ctx,foreignArrow({q:1,r:1,heading:'north-east'},px,live));
 assert.ok(calls.length>0);assert.ok(calls.every(c=>c.slice(0,4).every(Number.isInteger)));
 assert.ok(new Set(calls.map(c=>c[4])).size<=4);
});

test('weighted sprites and trails follow server PositionAt entry ownership, legacy couriers follow ReadMarch round',()=>{
 const costs=[750,2500,750];
 assert.equal(routePosition(path,-.1,px,costs,6).index,0);
 assert.equal(routePosition(path,0,px,costs,6).index,1);
 const p=routePosition(path,1/3,px,costs,6);assert.deepEqual([p.q,p.r],[1,1]);assert.deepEqual([p.x,p.y],[10,10]);
 assert.equal(routePosition(path,.1874,px,costs,6).index,1);assert.equal(routePosition(path,.1875,px,costs,6).index,2);
 assert.deepEqual(remainingRoute(path,.9,px,costs,6),[],'already reached target has no forward route before executed arrival');
 const courier=routePosition(path,.5,px);assert.deepEqual([courier.q,courier.r],[1,1]);assert.deepEqual([courier.x,courier.y],[10,5]);
});
test('arrow border and route shadow pixels are also clipped to LIVE sight',()=>{
 const calls=[],ctx={save(){},restore(){},fillRect(x,y,w,h){calls.push([x,y,w,h]);}};
 drawMovement(ctx,foreignArrow({q:1,r:1,heading:'north-east'},px,live),x=>x<21);
 assert.ok(calls.length);assert.ok(calls.every(c=>c[0]<21&&c[2]===1&&c[3]===1));
});

test('version2 begins at actual departure and reaches each hex at its own prefix',()=>{
 const a={route_version:2,dispatch_offset_milli:733,departure_tick:103,arrival_tick:104};
 const p=routeProgress(a,0,103.733);
 assert.ok(Math.abs(p)<1e-12);
 for(const [progress,index] of [[0,0],[.332,0],[.333,1],[.666,1],[.667,2],[.999,2],[1,3]]) {
  assert.equal(routePosition(path,progress,px,[200,200,200],1,2).index,index);
 }
});

test('a runner land leg uses its own departure rather than original correspondence send',()=>{
 const leg={route_version:2,sent_at:'2026-10-25T00:00:00Z',leg_start:'2026-10-25T12:00:00Z',leg_end:'2026-10-25T12:02:00Z',arrives_at:'2026-10-25T12:02:00Z'};
 const progress=routeProgress(leg,Date.parse('2026-10-25T12:00:30Z'),9999);
 assert.equal(progress,.25);
 assert.equal(routePosition(path,progress,px,[1,1,1],2,2).index,0);
});
