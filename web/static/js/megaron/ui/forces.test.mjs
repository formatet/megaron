import test from 'node:test';
import assert from 'node:assert/strict';
import { forceSections } from './forces.js';
import { warMovements } from './movements.js';

test('Forces groups by kind, not status; People exists only with a Nomadic Host',()=>{
 const forces=[{id:'land',type:'spearman',category:'land',status:'marching'},{id:'ship',type:'merchantman',category:'naval',status:'freighting'},{id:'laid',type:'galley',category:'naval',status:'laid_up'}];
 assert.deepEqual(forceSections(forces).map(g=>[g.name,g.forces.map(u=>u.id)]),[['Land Forces',['land']],['Ships',['ship','laid']]]);
 const withHost=forceSections([...forces,{id:'host',type:'nomadic_host',category:'land',status:'positioned'}]);assert.deepEqual(withHost[2].forces.map(u=>u.id),['host']);assert.equal(withHost[2].name,'People');
});
test('Marches excludes every bound trade ship, retaining ordered land and naval movement',()=>{
 const common={status:'marching',q:0,r:0,target_q:2,target_r:0};
 const {outgoing}=warMovements({units:[{...common,id:'ordered',display_name:'Ordered Spearmen'},{...common,id:'sail',category:'naval',display_name:'Ordered Galley'},{...common,id:'bound',category:'naval',display_name:'Route Ship',bound_route_id:'route'},{...common,id:'freight',category:'naval',status:'freighting',display_name:'Freighter'}]});
 assert.deepEqual(outgoing.map(m=>m.title),['Ordered Spearmen','Ordered Galley']);
});
