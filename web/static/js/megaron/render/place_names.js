// Generic geographic names come from Temenos, never from partial client chains.
import { namedRiverTerrain } from './river_names.js';
export const placeLabelOverlap=(a,b)=>a.left<b.right&&b.left<a.right&&a.top<b.bottom&&b.top<a.bottom;
export function namedPlaceTerrain(tile,fallback){
 if(tile.terrain==='fog'||tile.tier==='fog'||tile.visible===false)return fallback;
 if(tile.place_name)return tile.place_name;
 if(tile.place_kind==='river'&&!tile.river_name)return {river:'River',river_ford:'River Ford',river_delta:'River Delta'}[tile.terrain]||fallback;
 if(tile.place_kind==='massif')return 'Mountains';
 if(tile.place_kind==='desert')return 'Desert';
 if(tile.place_kind==='forest')return 'Cedar Forest';
 return namedRiverTerrain(tile,fallback);
}
export const PLACE_LABEL_VARIANTS={
 a:{font:'italic 7px serif',spacing:0,height:9},
 b:{font:'6px serif',spacing:1,height:8},
};
export function placeLabelVariant(search=''){return new URLSearchParams(search).get('ortnamn')==='b'?'b':'a';}
// World-space AABBs conservatively contain rotated text and clearance. These
// are collision facts, not a substitute for Timothy's visual choice.
export function placeLabelLayout(tiles,provinces,{hexPx,measure,measureCity,zoom,viewport,variant='a'}){
 if(zoom<.55)return [];
 const style=PLACE_LABEL_VARIANTS[variant]||PLACE_LABEL_VARIANTS.a;
 const occupied=[];
 for(const p of provinces){const {x,y}=hexPx(p.q,p.r),w=measureCity(p.name,!!p.own)+4;occupied.push({left:x-w/2,right:x+w/2,top:y+17,bottom:y+29},{left:x-24,right:x+24,top:y-30,bottom:y+18});}
 const groups=new Map();
 for(const t of tiles){if(!t.place_name||!t.place_kind||!t.place_id||t.terrain==='fog'||t.tier==='fog'||t.visible===false)continue;const key=t.place_kind+':'+t.place_id;if(!groups.has(key))groups.set(key,[]);groups.get(key).push(t);}
 const out=[];
 for(const [id,group] of [...groups].sort(([a],[b])=>a.localeCompare(b))){
  const points=group.map(t=>({...t,...hexPx(t.q,t.r)}));
  const cx=points.reduce((n,t)=>n+t.x,0)/points.length,cy=points.reduce((n,t)=>n+t.y,0)/points.length;
  points.sort((a,b)=>Math.hypot(a.x-cx,a.y-cy)-Math.hypot(b.x-cx,b.y-cy)||a.q-b.q||a.r-b.r);
  const name=group[0].place_name,w=measure(name,style)+Math.max(0,[...name].length-1)*style.spacing+4,h=style.height+4;
  let chosen;
  for(const t of points){
   // A river follows the local known channel. Other kinds lie over the known
   // area's middle. Sorting and canonical IDs make reloads deterministic.
   const neighbours=points.filter(n=>n!==t&&Math.max(Math.abs(n.q-t.q),Math.abs(n.r-t.r),Math.abs(n.q+n.r-t.q-t.r))===1).sort((a,b)=>a.q-b.q||a.r-b.r);
   let angle=0;
   if(t.place_kind==='river'&&neighbours.length){const a=neighbours[0],b=neighbours.length>1?neighbours.at(-1):t;angle=Math.atan2(b.y-a.y,b.x-a.x);if(angle>Math.PI/2)angle-=Math.PI;if(angle<-Math.PI/2)angle+=Math.PI;}
   const rw=Math.abs(w*Math.cos(angle))+Math.abs(h*Math.sin(angle)),rh=Math.abs(w*Math.sin(angle))+Math.abs(h*Math.cos(angle));
   for(const dy of [0,-14,14,-26,26]){const rect={left:t.x-rw/2,right:t.x+rw/2,top:t.y+dy-rh/2,bottom:t.y+dy+rh/2};if(rect.left<viewport.left||rect.right>viewport.right||rect.top<viewport.top||rect.bottom>viewport.bottom||occupied.some(o=>placeLabelOverlap(rect,o)))continue;chosen={id,kind:t.place_kind,name,x:t.x,y:t.y+dy,angle,rect};break;}
   if(chosen)break;
  }
  if(chosen){out.push(chosen);occupied.push(chosen.rect);}
 }
 return out;
}
