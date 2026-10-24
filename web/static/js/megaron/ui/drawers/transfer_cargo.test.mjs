import test from 'node:test';
import assert from 'node:assert/strict';
import { cargoStatus, transferItems } from './economy.js';

test('cargo counts every quantity equally and shows total overload', () => {
  const items = [{good_key:'grain',quantity:12},{good_key:'timber',quantity:9}];
  assert.deepEqual(cargoStatus(items,{category:'naval',ship_name:'Naia',ship_type:'galley',cargo_capacity:20}),
    {quantity:21,overloaded:true,text:'Naia · Cargo 21 / 20 — remove 1 goods'});
  assert.equal(cargoStatus(items,{category:'land'}).text,'Land caravan · Cargo 21 — no cargo limit');
  assert.equal(cargoStatus([],{category:'naval',ship_type:'merchantman',cargo_capacity:120}).text,'Emporos · Cargo 0 / 120');
});
test('manifest inputs keep positive fractional amounts and omit empty rows', () => {
  assert.deepEqual(transferItems([{dataset:{good:'grain'},value:'2.5'},{dataset:{good:'timber'},value:''}]),[{good_key:'grain',quantity:2.5}]);
});
import {notifText} from '../format.js';
import {renderCargoHTML} from './economy.js';
test('list outcomes and cargo table show every good without exposing sender city',()=>{
 const body={sender_name:'Myrine',recipient_name:'Leandros',origin_name:'Secret city',items:[{good_key:'grain',quantity:12,credited_quantity:6,lost_quantity:6},{good_key:'timber',quantity:8,credited_quantity:4,lost_quantity:4}]};
 const text=notifText('GiftDeliveredV2',body);
 for(const value of ['Myrine → Leandros','6 grain received; 6 lost','4 timber received; 4 lost']) assert.ok(text.includes(value),text);
 assert.doesNotMatch(text,/Secret city/);
 const html=renderCargoHTML([{items:body.items,arrives_at:'2030-01-01',origin_name:'Lato',dest_name:'Gournia'}],0);
 assert.match(html,/grain/);assert.match(html,/timber/);
});
