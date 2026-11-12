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

// ── Desktop: a window that opens on top of another closes it (through that window's own close) ──
const {rectsOverlap}=await import('./mobile_panels.js');
const box=(l,t,r,b)=>({left:l,top:t,right:r,bottom:b,width:r-l,height:b-t});
const placed=(rect)=>{const e=element();e.getBoundingClientRect=()=>rect;return e;};
const settle=()=>new Promise(r=>setTimeout(r,0));
test('rectsOverlap: touching edges and empty boxes do not overlap',()=>{
 assert.equal(rectsOverlap(box(0,0,10,10),box(5,5,15,15)),true);
 assert.equal(rectsOverlap(box(0,0,10,10),box(10,0,20,10)),false);
 assert.equal(rectsOverlap(box(0,0,10,10),box(0,10,10,20)),false);
 assert.equal(rectsOverlap(box(0,0,0,0),box(0,0,10,10)),false);
 assert.equal(rectsOverlap(null,box(0,0,10,10)),false);
});
test('desktop: an opener that covers an open window closes it through its own callback; a window apart stays',async()=>{
 width=1280;const hexPanel=placed(box(1060,0,1280,700)),codex=placed(box(0,34,440,766)),drawer=placed(box(860,34,1280,766));
 let closedHex=0,closedCodex=0;
 openMobilePanel(hexPanel,()=>{closedHex++;closeMobilePanel(hexPanel)},{sheet:true});
 openMobilePanel(codex,()=>{closedCodex++;closeMobilePanel(codex)});
 await settle();assert.equal(closedHex,0,'codex is apart from the hex panel');assert.equal(closedCodex,0);
 openMobilePanel(drawer,()=>closeMobilePanel(drawer));
 await settle();assert.equal(closedHex,1,'the drawer covers the hex panel');assert.equal(closedCodex,0,'the drawer is apart from the codex');
 closeMobilePanel(codex);closeMobilePanel(drawer);
});
test('desktop: the newest window wins even when it sits beneath the old one (hex click while a drawer is open)',async()=>{
 width=1280;const drawer=placed(box(860,34,1280,766)),hexPanel=placed(box(1060,0,1280,700));let closed=0;
 openMobilePanel(drawer,()=>{closed++;closeMobilePanel(drawer)});await settle();
 openMobilePanel(hexPanel,()=>closeMobilePanel(hexPanel),{sheet:true});await settle();
 assert.equal(closed,1);closeMobilePanel(hexPanel);
});
test('desktop: a window closed before the check runs is not judged; an element without geometry never overlaps',async()=>{
 width=1280;const a=placed(box(0,0,100,100)),b=placed(box(0,0,100,100)),c=element();let closed=0;
 openMobilePanel(a,()=>{closed++;closeMobilePanel(a)});openMobilePanel(b,()=>closeMobilePanel(b));closeMobilePanel(b);
 openMobilePanel(c,()=>{});await settle();assert.equal(closed,0);closeMobilePanel(a);closeMobilePanel(c);
});
test('phone: opening a panel does not run the desktop overlap check',async()=>{
 width=390;const a=placed(box(0,0,100,100)),b=placed(box(0,0,100,100));let closed=0;
 openMobilePanel(a,()=>{closed++;closeMobilePanel(a)});openMobilePanel(b,()=>closeMobilePanel(b));
 assert.equal(closed,1,'phone closes the predecessor synchronously, as before');await settle();assert.equal(closed,1);
 closeMobilePanel(b);width=1000;
});
test('desktop: a popup menu (march menu) covers what it sits on without closing it, but a window opened over it closes it',async()=>{
 width=1280;const drawer=placed(box(860,34,1280,766)),menu=placed(box(900,300,1110,500)),dlg=placed(box(850,100,1100,400));let closedDrawer=0,closedMenu=0;
 openMobilePanel(drawer,()=>{closedDrawer++;closeMobilePanel(drawer)});await settle();
 openMobilePanel(menu,()=>{closedMenu++;closeMobilePanel(menu)},{popup:true});await settle();
 assert.equal(closedDrawer,0);assert.equal(closedMenu,0);
 openMobilePanel(dlg,()=>closeMobilePanel(dlg));await settle();
 assert.equal(closedMenu,1);assert.equal(closedDrawer,1);closeMobilePanel(dlg);
});
