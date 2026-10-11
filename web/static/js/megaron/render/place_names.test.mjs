import test from 'node:test';import assert from 'node:assert/strict';
import {namedPlaceTerrain,placeLabelLayout,placeLabelOverlap,placeLabelVariant} from './place_names.js';
const hexPx=(q,r)=>({x:q*33,y:(r+q/2)*38});
const opts={hexPx,measure:s=>s.length*4,measureCity:s=>s.length*4,zoom:1,viewport:{left:-400,top:-400,right:400,bottom:400}};
test('generic names and nameless terrain headers cannot leak through fog',()=>{
 for(const [kind,terrain,label] of [['river','river','River'],['massif','mountain_red','Mountains'],['desert','semi_desert','Desert'],['forest','forest_cedar','Cedar Forest']]){
  const t={place_kind:kind,terrain};assert.equal(namedPlaceTerrain(t,terrain),label);assert.equal(namedPlaceTerrain({...t,place_name:'Ariadne'},label),'Ariadne');assert.equal(namedPlaceTerrain({...t,tier:'fog',place_name:'Hidden'},'Unexplored'),'Unexplored');
 }
 assert.equal(placeLabelVariant('?ortnamn=b'),'b');assert.equal(placeLabelVariant('?ortnamn=a'),'a');
});
test('all four place labels occur once, avoid measured city boxes, fog and unreadable zoom in both variants',()=>{
 const tiles=[];for(const [i,kind] of ['river','massif','desert','forest'].entries())for(let q=0;q<5;q++)tiles.push({q:q-2,r:i*3-5,terrain:kind==='river'?'river':'forest_cedar',place_kind:kind,place_id:`${i},0`,place_name:['Thaleia','Mount Ida','Damon Sands','Ariadne Cedars'][i],tier:'remembered'});
 tiles.push({q:0,r:0,terrain:'fog',place_kind:'forest',place_id:'hidden',place_name:'Hidden',tier:'fog'});
 const provinces=[{q:0,r:1,name:'Mycenae',own:true}];
 for(const variant of ['a','b']){const labels=placeLabelLayout(tiles,provinces,{...opts,variant});assert.equal(labels.length,4);assert.equal(new Set(labels.map(l=>l.id)).size,4);assert.ok(!labels.some(l=>l.name==='Hidden'));assert.ok(labels.some(l=>l.kind==='river'&&l.angle!==0));
  const p=hexPx(0,1),city={left:p.x-18,right:p.x+18,top:p.y+17,bottom:p.y+29};assert.ok(labels.every(l=>!placeLabelOverlap(l.rect,city)));const cityArt={left:p.x-24,right:p.x+24,top:p.y-30,bottom:p.y+18};assert.ok(labels.every(l=>!placeLabelOverlap(l.rect,cityArt)),'place label must yield to city icon at its preferred centre');for(let i=0;i<labels.length;i++)for(let j=i+1;j<labels.length;j++)assert.ok(!placeLabelOverlap(labels[i].rect,labels[j].rect));
  assert.deepEqual(placeLabelLayout([...tiles].reverse(),provinces,{...opts,variant}),labels);assert.deepEqual(placeLabelLayout(tiles,provinces,{...opts,variant,zoom:.3}),[]);
 }
});
