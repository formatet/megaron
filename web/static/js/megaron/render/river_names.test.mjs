import test from 'node:test';
import assert from 'node:assert/strict';
import { namedRiverTerrain, riverLabelLayout, riverLabelOverlap } from './river_names.js';
const hexPx=(q,r)=>({x:q*33,y:(r+q/2)*38});
const opts={hexPx,measure:s=>s.length*4,zoom:1,viewport:{left:-300,top:-300,right:300,bottom:300}};
test('river panel names river, ford and delta; fog cannot reveal a supplied name',()=>{
  for(const [terrain,want] of [['river','River Alpheios'],['river_ford','Ford of Alpheios'],['river_delta','Delta of Alpheios']]) {
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios'},terrain),want);
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios',tier:'remembered'},terrain),want);
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios',tier:'fog'},'Unexplored'),'Unexplored');
  }
  assert.equal(namedRiverTerrain({terrain:'plains',river_name:'Alpheios'},'Plains'),'Plains');
});
test('river labels appear once across known fragments and avoid city labels, fog and minzoom',()=>{
 const tiles=[0,1,2,3,4].map(q=>({q,r:0,terrain:'river',river_name:'Alpheios',tier:q===2?'fog':'live'}));
 tiles.push({q:0,r:-3,terrain:'river_ford',river_name:'Eurotas',tier:'remembered'});
 tiles.push({q:0,r:3,terrain:'fog',river_name:'Hidden'});
 const provinces=[{q:2,r:0,name:'Mycenae',own:true}];
 const labels=riverLabelLayout(tiles,provinces,opts);
 assert.deepEqual(labels.map(l=>l.name),['Alpheios','Eurotas']);
 const c=hexPx(2,0);const city={left:c.x-16,right:c.x+16,top:c.y+17,bottom:c.y+29};
 assert.ok(labels.every(l=>!riverLabelOverlap(l.rect,city)));
 assert.ok(!riverLabelOverlap(labels[0].rect,labels[1].rect));
 assert.deepEqual(riverLabelLayout(tiles,provinces,{...opts,zoom:.3}),[]);
 assert.deepEqual(riverLabelLayout([...tiles].reverse(),provinces,opts),labels,'tile order cannot move names');
});
