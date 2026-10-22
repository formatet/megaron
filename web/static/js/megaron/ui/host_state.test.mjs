import test from 'node:test';
import assert from 'node:assert/strict';
import {physicalHostState,hostStatusText} from './host_state.js';

test('Host uses physical server projection, never stored departure; latest tick projection wins',()=>{
 const phase={active:true,host_unit_id:'host',q:0,r:0,host_state:{q:2,r:0,at_milli:3000,status:'marching',can_found_here:true}};
 const unit={id:'host',type:'nomadic_host',status:'marching',q:0,r:0,host_state:{q:3,r:0,at_milli:4000,status:'marching',can_found_here:true}};
 assert.deepEqual(physicalHostState(phase,[unit]),unit.host_state);
 phase.host_state={...phase.host_state,q:4,at_milli:5000};
 assert.deepEqual(physicalHostState(phase,[unit]),phase.host_state);
 assert.equal(physicalHostState({...phase,active:false},[unit]),null,'dissolved phase has no marker');
 assert.equal(physicalHostState({active:true,host_unit_id:'host',q:0,r:0},[{...unit,host_state:null}]),null,'missing physical projection cannot pretend departure is current');
 assert.equal(hostStatusText({status:'marching'}),'Marching');
 assert.equal(hostStatusText({status:'can_found_here'}),'Can found here');
 assert.equal(hostStatusText({status:'waiting',reason:'This ground is occupied.'}),'Waiting — This ground is occupied.');
});
