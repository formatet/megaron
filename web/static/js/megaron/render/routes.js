// Moving actors share one remaining-route renderer. Foreign projections are
// deliberately read only as a current position and a compass, never as a route.
import { FOREIGN_ACCENT, INTENT_ACCENT } from './actorsprites.js';
export const ROUTE_PATTERNS={army:[],ship:[4,3],runner:[1,3],caravan:[5,2,1,2]};
const COMPASS={north:[0,-1],'north-east':[1,-1],east:[1,0],'south-east':[1,1],south:[0,1],'south-west':[-1,1],west:[-1,0],'north-west':[-1,-1]};
const clamp=n=>Math.max(0,Math.min(1,n));
export function routeProgress(actor,nowMs,tickNow=null){
 if(actor.route_version===2){const a=Date.parse(actor.leg_start||actor.departs_at||actor.sent_at),b=Date.parse(actor.leg_end||actor.arrives_at);if(Number.isFinite(a)&&b>a)return Math.min(1,(nowMs-a)/(b-a));}
 const start=actor.departure_tick,end=actor.arrival_tick;
 if(tickNow!=null&&Number.isFinite(start)&&Number.isFinite(end)&&end>start)return Math.min(1,(tickNow-start-(actor.dispatch_offset_milli||0)/1000)/(end-start));
 const a=Date.parse(actor.leg_start||actor.departs_at||actor.sent_at),b=Date.parse(actor.leg_end||actor.arrives_at);
 return Number.isFinite(a)&&b>a?Math.min(1,(nowMs-a)/(b-a)):null;
}
export function routePosition(path,progress,hexPx,costs=null,durationTicks=null,version=0){
 if(!Array.isArray(path)||!path.length||progress==null)return null;
 const p=clamp(progress);
 if(path.length===1||p===1){const h=path.at(-1);return {...hexPx(...h),q:h[0],r:h[1],index:path.length-1,local:1};}
 const weighted=Array.isArray(costs)&&costs.length===path.length-1&&costs.every(c=>Number.isFinite(c)&&c>0);
 if(weighted){
  // movement.PositionAt (val A): at departure the FIRST entered hex owns
  // the unit. Each cost is time spent in that entered hex, not the previous
  // edge. Keep its centre and FOW hex identical to the server's province.
  const total=costs.reduce((a,b)=>a+b,0),duration=durationTicks>0?durationTicks*1000:1000000;
  const at=Math.floor(p*duration);let index=version===2?0:(progress<0?0:1),prefix=0;
  for(let i=1;i<costs.length+(version===2?1:0)&&progress>=0;i++){
   prefix+=costs[i-1];const boundary=Math.floor((duration*prefix+Math.floor(total/2))/total);
   if(at>=boundary)index=i+(version===2?0:1);else break;
  }
  const h=path[index];return {...hexPx(...h),q:h[0],r:h[1],index,local:0};
 }
 // Legacy couriers use ReadMarch's uniform interpolation and rounded hex;
 // the continuous pixel itself is retained for their gait between centres.
 const steps=p*(path.length-1),index=Math.min(path.length-2,Math.floor(steps)),local=steps-index;
 const a=hexPx(...path[index]),b=hexPx(...path[index+1]),h=path[Math.round(steps)];
 return {x:a.x+(b.x-a.x)*local,y:a.y+(b.y-a.y)*local,q:h[0],r:h[1],index,local};
}
export function remainingRoute(path,progress,hexPx,costs=null,durationTicks=null,version=0){
 const pos=routePosition(path,progress,hexPx,costs,durationTicks,version);
 if(!pos||pos.index>=path.length-1)return [];
 return [pos,...path.slice(pos.index+1).map(h=>({...hexPx(...h),q:h[0],r:h[1]}))];
}
export function liveRouteSegments(points,live){
 const out=[];
 for(let i=1;i<points.length;i++)if(live(points[i-1].q,points[i-1].r)&&live(points[i].q,points[i].r))out.push([points[i-1],points[i]]);
 return out;
}
export function foreignArrow(actor,hexPx,live,kind='army'){
 const q=actor.current_q??actor.q,r=actor.current_r??actor.r;
 if(q==null||r==null||!live(q,r)||!COMPASS[actor.heading])return null;
 return {type:'arrow',kind,...(actor.id?{actor_id:actor.id}:{}),...hexPx(q,r),heading:actor.heading};
}
export function actorRoute(actor,kind,nowMs,tickNow,hexPx,live){
 const points=remainingRoute(actor.path,routeProgress(actor,nowMs,tickNow),hexPx,actor.step_costs,actor.arrival_tick-actor.departure_tick,actor.route_version);
 const segments=liveRouteSegments(points,live);
 return segments.length?{type:'route',kind,...(actor.id?{actor_id:actor.id}:{}),segments}:null;
}
// Pixel rasterization avoids blurred canvas strokes. Dash phase is shared
// across a route's corners; a short dark shadow keeps signal on light ground.
function pixels(a,b){
 let x=Math.round(a.x),y=Math.round(a.y),tx=Math.round(b.x),ty=Math.round(b.y);
 const dx=Math.abs(tx-x),sx=x<tx?1:-1,dy=-Math.abs(ty-y),sy=y<ty?1:-1;
 let err=dx+dy;const out=[];
 for(;;){out.push([x,y]);if(x===tx&&y===ty)break;const e=2*err;if(e>=dy){err+=dy;x+=sx;}if(e<=dx){err+=dx;y+=sy;}}
 return out;
}
function patterned(index,pattern){
 if(!pattern.length)return true;
 let n=index%pattern.reduce((a,b)=>a+b,0);
 for(let i=0;i<pattern.length;i++){if(n<pattern[i])return i%2===0;n-=pattern[i];}
 return false;
}
export function drawMovement(ctx,primitive,livePixel=()=>true){
 const dot=(x,y)=>{if(livePixel(x,y))ctx.fillRect(x,y,1,1);};
 ctx.save();
 if(primitive.type==='route'){
  const pattern=ROUTE_PATTERNS[primitive.kind]||ROUTE_PATTERNS.army;let phase=0;const ink=[];
  for(const [a,b] of primitive.segments){for(const p of pixels(a,b)){if(patterned(phase++,pattern))ink.push(p);}}
  ctx.fillStyle='#241F18';for(const [x,y] of ink)dot(x,y+1);
  ctx.fillStyle=primitive.relation==='foreign'?FOREIGN_ACCENT:INTENT_ACCENT.explore;for(const [x,y] of ink)dot(x,y);
 }else{
  const d=COMPASS[primitive.heading],length=Math.hypot(...d),dx=d[0]/length,dy=d[1]/length;
  const cx=Math.round(primitive.x+12),cy=Math.round(primitive.y-12),tip={x:cx+dx*4,y:cy+dy*4};
  const ink=[...pixels({x:cx-dx*3,y:cy-dy*3},tip),...pixels(tip,{x:tip.x-dx*3-dy*3,y:tip.y-dy*3+dx*3}),...pixels(tip,{x:tip.x-dx*3+dy*3,y:tip.y-dy*3-dx*3})];
  ctx.fillStyle='#E8D0A8';for(const [x,y] of ink)for(let dx=-1;dx<=1;dx++)for(let dy=-1;dy<=1;dy++)dot(x+dx,y+dy);
  ctx.fillStyle=FOREIGN_ACCENT;for(const [x,y] of ink)dot(x,y);
 }
 ctx.restore();
}
