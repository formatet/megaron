// Shared server tick anchor; no DOM/UI dependencies.
import {State} from './state.js';
import {serverNow} from './clock.js';
// Use the server's actual tick boundary, not the instant its reply arrived.
export function anchorWorldTime(world,nowMs=serverNow(),state=State){
 if(!Number.isFinite(world.current_tick)||!(world.tick_seconds>0))return false;
 state.CURRENT_TICK=world.current_tick;state.TICK_SECONDS=world.tick_seconds;
 const anchor=Date.parse(world.tick_anchor_at);
 state.TICK_ANCHOR_MS=Number.isFinite(anchor)?anchor:nowMs;
 return true;
}
export function worldTickNow(nowMs=serverNow(),state=State){
 if(!Number.isFinite(state.CURRENT_TICK)||!(state.TICK_SECONDS>0)||!Number.isFinite(state.TICK_ANCHOR_MS))return null;
 return state.CURRENT_TICK+(nowMs-state.TICK_ANCHOR_MS)/(state.TICK_SECONDS*1000);
}

// Physical movement follows tick.Anchor.MilliAt's current-tick clamp. A
// stalled worker must not let a client predict a unit into its next tick.
export function movementTickNow(nowMs=serverNow(),state=State){
 const tick=worldTickNow(nowMs,state);
 return tick==null?null:state.CURRENT_TICK+Math.min(.999,Math.max(0,Math.floor((tick-state.CURRENT_TICK)*1000)/1000));
}

// Remaining wall time from the authoritative world tick and cadence.
export function msUntilWorldTick(arrivalTick,nowMs=serverNow(),state=State){
 const current=worldTickNow(nowMs,state);
 return Number.isFinite(arrivalTick)&&current!=null?(arrivalTick-current)*state.TICK_SECONDS*1000:NaN;
}
