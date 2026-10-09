import test from 'node:test';
import assert from 'node:assert/strict';
import { bindMapInput } from './map_input.js';
import { zoomStep } from './camera.js';
import { ZOOM_MIN, ZOOM_MAX } from '../config.js';
function rig() {
  const events={}, captured=new Set(), calls=[], timers=new Map();let serial=0;
  const canvas={addEventListener(k,fn){events[k]=fn;},setPointerCapture(id){captured.add(id);},hasPointerCapture(id){return captured.has(id);},releasePointerCapture(id){captured.delete(id);}};
  const callbacks=Object.fromEntries(['pan','zoom','tap','orders','hover','hideHover'].map(k=>[k,(...args)=>calls.push([k,...args])]));
  const clear=bindMapInput(canvas,callbacks,{schedule(fn,ms){assert.equal(ms,500);timers.set(++serial,fn);return serial;},cancel(id){timers.delete(id);}});
  const fire=(kind,x=100,y=100,id=1,type='touch',extra={})=>events[kind]({clientX:x,clientY:y,pointerId:id,pointerType:type,button:0,preventDefault(){},...extra});
  const tick=()=>{const fs=[...timers.values()];timers.clear();fs.forEach(f=>f());};
  return {events,captured,calls,timers,callbacks,clear,fire,tick,of:k=>calls.filter(c=>c[0]===k)};
}
test('U one pointer path registers no legacy mouse/touch handlers',()=>{
 const r=rig();for(const k of ['mousedown','mouseup','mousemove','touchstart','touchmove'])assert.equal(r.events[k],undefined);
 for(const k of ['pointerdown','pointermove','pointerup','pointercancel','lostpointercapture','contextmenu','wheel'])assert.equal(typeof r.events[k],'function');
});
for(const type of ['mouse','touch','pen']){
 test(`U ${type}: tap selects once; drag pans and never selects even on release at last move`,()=>{
  const r=rig();r.fire('pointerdown',100,100,1,type);assert.ok(r.captured.has(1));r.fire('pointerup',100,100,1,type);assert.equal(r.of('tap').length,1);assert.equal(r.captured.size,0);
  r.fire('pointerdown',100,100,1,type);r.fire('pointermove',140,160,1,type);r.fire('pointerup',140,160,1,type);assert.deepEqual(r.of('pan'),[['pan',40,60]]);assert.equal(r.of('tap').length,1);r.tick();assert.equal(r.of('orders').length,0);
 });
}
test('U long touch/pen opens orders once, release never selects, browser menu is prevented',()=>{
 for(const type of ['touch','pen']){
  const r=rig();r.fire('pointerdown',100,100,1,type);assert.equal(r.of('orders').length,0);r.tick();assert.deepEqual(r.of('orders'),[['orders',100,100]]);
  let prevented=false;r.fire('contextmenu',100,100,1,type,{preventDefault(){prevented=true;}});assert.ok(prevented);r.fire('pointerup',100,100,1,type);
  // Older engines may send the native touch contextmenu as a MouseEvent after up.
  r.fire('contextmenu',100,100,1,'touch',{pointerType:undefined});assert.equal(r.of('orders').length,1);assert.equal(r.of('tap').length,0);
 }
});
test('U motion cancels long press even if finger returns to its start',()=>{
 const r=rig();r.fire('pointerdown');r.fire('pointermove',110,100);r.fire('pointermove',100,100);r.tick();r.fire('pointerup');assert.equal(r.of('orders').length,0);assert.equal(r.of('tap').length,0);
});
test('U pinch uses moving midpoint, cancels long press and never becomes a tap after one finger lifts',()=>{
 const r=rig();r.fire('pointerdown',100,100);r.fire('pointerdown',200,100,2);r.fire('pointermove',70,100);r.fire('pointermove',230,100,2);r.tick();
 assert.deepEqual(r.of('zoom'),[['zoom',1.3,150,100,-15,0],['zoom',160/130,135,100,15,0]]);
 r.fire('pointerup',70,100);r.fire('pointermove',240,100,2);r.fire('pointerup',240,100,2);assert.equal(r.of('tap').length,0);assert.equal(r.of('orders').length,0);assert.deepEqual(r.of('pan'),[['pan',10,0]]);
});
test('U pinch and wheel share zoom clamps and preserve the world anchor at both limits',()=>{
 const r=rig();let camera={x:20,y:30,zoom:1};r.callbacks.zoom=(factor,x,y,dx,dy)=>{const n=zoomStep(camera,factor,x,y);camera={...n,x:n.x+dx,y:n.y+dy};};
 // callbacks are supplied by the map; bind a second rig with a real camera adapter.
 const events={};bindMapInput({addEventListener(k,f){events[k]=f;}},{...r.callbacks});
 const wheel=deltaY=>events.wheel({clientX:100,clientY:100,deltaY,preventDefault(){}});
 for(let i=0;i<100;i++)wheel(-1);assert.equal(camera.zoom,ZOOM_MAX);const max={...camera};wheel(-1);assert.deepEqual(camera,max);
 for(let i=0;i<100;i++)wheel(1);assert.equal(camera.zoom,ZOOM_MIN);const min={...camera};wheel(1);assert.deepEqual(camera,min);
 // Pinch calls the same zoom callback; extremes clamp before translating.
 events.pointerdown({pointerId:1,pointerType:'mouse',button:0,clientX:90,clientY:100});events.pointerdown({pointerId:2,pointerType:'mouse',button:0,clientX:110,clientY:100});events.pointermove({pointerId:2,pointerType:'mouse',clientX:100000,clientY:100});assert.equal(camera.zoom,ZOOM_MAX);
});
for(const kind of ['pointercancel','lostpointercapture'])test(`U ${kind} cancels selection and pending long press`,()=>{
 const r=rig();r.fire('pointerdown');r.fire(kind);r.tick();r.fire('pointerup');assert.equal(r.of('tap').length,0);assert.equal(r.of('orders').length,0);assert.equal(r.captured.size,0);
});
test('U blur/hidden cleanup releases all captures and cancels timers; a fresh gesture still works',()=>{
 const r=rig();r.fire('pointerdown');r.clear();r.tick();r.fire('pointerup');assert.equal(r.of('orders').length,0);assert.equal(r.of('tap').length,0);assert.equal(r.captured.size,0);r.fire('pointerdown');r.fire('pointerup');assert.equal(r.of('tap').length,1);
});
test('U mouse hover, leave, right-click and wheel retain their routes; right down never starts drag',()=>{
 const r=rig();r.fire('pointermove',20,30,1,'mouse');assert.deepEqual(r.of('hover'),[['hover',20,30]]);r.fire('pointerleave');assert.equal(r.of('hideHover').length,1);
 r.fire('pointerdown',20,30,1,'mouse',{button:2});r.fire('contextmenu',20,30,1,'mouse');assert.deepEqual(r.of('orders'),[['orders',20,30]]);assert.equal(r.captured.size,0);
 r.fire('wheel',20,30,1,'mouse',{deltaY:-10});r.fire('wheel',20,30,1,'mouse',{deltaY:10});assert.deepEqual(r.of('zoom'),[['zoom',1.1,20,30,0,0],['zoom',.91,20,30,0,0]]);
});
