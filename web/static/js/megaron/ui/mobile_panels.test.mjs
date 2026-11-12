import test from 'node:test';
import assert from 'node:assert/strict';
const node=()=>({classes:new Set(),classList:{add(c){this.owner.classes.add(c)},remove(c){this.owner.classes.delete(c)}}});
function element(){const e=node();e.classList.owner=e;return e;}
let width=390;
const canvas=element(),map=element(),body=element();
globalThis.window={matchMedia:()=>({matches:width<=480}),addEventListener(){}};
globalThis.document={body,getElementById:id=>id==='hex-canvas'?canvas:id==='map-root'?map:null};
const {openMobilePanel,closeMobilePanel,refreshMobilePanels}=await import('./mobile_panels.js');
test('all mobile surfaces fill screen, close predecessor via cleanup, and restore map touch on close',()=>{
 let previous=null,closed=0;
 for(const surface of ['city','war','diplomacy','economy','kult','notif','inspect','march','dispatch','account','search','codex']){
  const el=element();
  openMobilePanel(el,()=>{closed++;closeMobilePanel(el)});
  assert.ok(el.classes.has('mobile-fullscreen'),surface);
  if(previous)assert.ok(!previous.classes.has('mobile-fullscreen'),surface+' closed predecessor');
  assert.equal(canvas.inert,true);assert.ok(map.classes.has('mobile-panel-blocked'));
  previous=el;
 }
 assert.equal(closed,11);
 closeMobilePanel(previous);
 assert.equal(canvas.inert,false);assert.ok(!body.classes.has('mobile-panel-open'));
});
test('desktop preserves simultaneous windows; shrinking keeps last and widening restores desktop classes',()=>{
 width=1000;const a=element(),b=element();let closed=0;
 openMobilePanel(a,()=>{closed++;closeMobilePanel(a)});openMobilePanel(b,()=>closeMobilePanel(b));
 assert.equal(canvas.inert,false);assert.equal(closed,0);assert.ok(!b.classes.has('mobile-fullscreen'));
 width=390;refreshMobilePanels();assert.equal(closed,1);assert.ok(b.classes.has('mobile-fullscreen'));assert.equal(canvas.inert,true);
 width=1000;refreshMobilePanels();assert.ok(!b.classes.has('mobile-fullscreen'));assert.equal(canvas.inert,false);closeMobilePanel(b);
});
const {sheetPanDelta}=await import('./mobile_panels.js');
test('a sheet does not block the map; a full-screen panel opened afterwards closes it; only sheets never mark body',()=>{
 width=390;const sheet=element(),full=element();let closed=0;
 openMobilePanel(sheet,()=>{closed++;closeMobilePanel(sheet)},{sheet:true});
 assert.ok(sheet.classes.has('mobile-sheet'));assert.ok(!sheet.classes.has('mobile-fullscreen'));
 assert.equal(canvas.inert,false);assert.ok(!map.classes.has('mobile-panel-blocked'));assert.ok(!body.classes.has('mobile-panel-open'));
 // another sheet replaces content without a close cycle
 openMobilePanel(sheet,()=>{closed++},{sheet:true});assert.equal(closed,0);assert.ok(sheet.classes.has('mobile-sheet'));
 openMobilePanel(full,()=>closeMobilePanel(full));
 assert.equal(closed,1);assert.ok(!sheet.classes.has('mobile-sheet'));assert.ok(full.classes.has('mobile-fullscreen'));
 assert.equal(canvas.inert,true);assert.ok(map.classes.has('mobile-panel-blocked'));
 closeMobilePanel(full);assert.equal(canvas.inert,false);assert.ok(!body.classes.has('mobile-panel-open'));
});
test('at desktop width a sheet gets neither sheet nor fullscreen class; shrinking applies the sheet, widening removes it',()=>{
 width=1000;const s=element();openMobilePanel(s,()=>closeMobilePanel(s),{sheet:true});
 assert.ok(!s.classes.has('mobile-sheet'));assert.ok(!s.classes.has('mobile-fullscreen'));assert.equal(canvas.inert,false);
 width=390;refreshMobilePanels();assert.ok(s.classes.has('mobile-sheet'));assert.equal(canvas.inert,false);
 width=1000;refreshMobilePanels();assert.ok(!s.classes.has('mobile-sheet'));closeMobilePanel(s);
 assert.ok(!s.classes.has('mobile-sheet'));
});
test('sheetPanDelta: 0 when the hex is already above the sheet, else the upward move that clears it',()=>{
 // map 600 px tall, sheet 300 px: free area is y < 300; pad keeps the whole hex (radius 40) plus margin visible
 assert.equal(sheetPanDelta(100,600,300,48),0);
 assert.equal(sheetPanDelta(252,600,300,48),0);
 assert.equal(sheetPanDelta(253,600,300,48),-1);
 assert.equal(sheetPanDelta(500,600,300,48),-248);
 assert.equal(sheetPanDelta(500,600,0,48),0);
 // a hex above the top edge is not dragged down by the sheet logic
 assert.equal(sheetPanDelta(-20,600,300,48),0);
});
