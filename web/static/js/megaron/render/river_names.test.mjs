import test from 'node:test';
import assert from 'node:assert/strict';
import { namedRiverTerrain } from './river_names.js';
test('river panel names river, ford and delta; fog cannot reveal a supplied name',()=>{
  for(const [terrain,want] of [['river','River Alpheios'],['river_ford','Ford of Alpheios'],['river_delta','Delta of Alpheios']]) {
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios'},terrain),want);
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios',tier:'remembered'},terrain),want);
    assert.equal(namedRiverTerrain({terrain,river_name:'Alpheios',tier:'fog'},'Unexplored'),'Unexplored');
  }
  assert.equal(namedRiverTerrain({terrain:'plains',river_name:'Alpheios'},'Plains'),'Plains');
});
