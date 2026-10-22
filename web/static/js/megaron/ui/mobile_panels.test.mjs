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
