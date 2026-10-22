import test from 'node:test';
import assert from 'node:assert/strict';
import {destinationCities,destinationCityOptions,destinationFromCity,cityDestinationLabel} from './city_targets.js';

test('city shortcut uses only exact Known entries, own first; duplicate names stay distinct',()=>{
 const cities=destinationCities([
  {settlement_id:'foreign',name:'Argos',knowledge:'known',q:4,r:0,owner:'Nestor'},
  {settlement_id:'rumour',name:'Troy',knowledge:'rumour',q:10,r:0},
  {settlement_id:'no-point',name:'Unknown location',knowledge:'known'},
  {settlement_id:'own',name:'Argos',knowledge:'known',q:0,r:0,own:true},
 ]);
 assert.deepEqual(cities.map(c=>c.settlement_id),['own','foreign']);
 const html=destinationCityOptions(cities);
 assert.match(html,/<optgroup label="Your cities">/);
 assert.match(html,/value="foreign">Argos — Nestor \(4,0\)/);
 assert.doesNotMatch(html,/Troy|Unknown location/);
 assert.deepEqual(destinationFromCity(cities[1]),{q:4,r:0,terrain:undefined,isSea:false,known:true,name:'Argos',isSettlement:true,allied:false,settlementID:'foreign'});
 assert.equal(cityDestinationLabel(destinationFromCity(cities[1])),' to Argos');
 assert.equal(cityDestinationLabel({q:4,r:0,name:'plains'}),'');
});
test('city names and ids cannot inject option markup',()=>{
 const html=destinationCityOptions(destinationCities([{settlement_id:'" onclick="attack',name:'<img src=x>',owner:'<script>',knowledge:'known',q:4,r:0}]));
 assert.doesNotMatch(html,/<img|<script|value="" onclick/);
 assert.match(html,/&lt;img/);
 assert.deepEqual(destinationCities(null),[]);
});
