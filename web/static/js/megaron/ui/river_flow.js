// The only hidden-neighbour information admitted here is the server's edge/direction.
export const RIVER_OFFSETS=[[1,0],[-1,0],[0,1],[0,-1],[1,-1],[-1,1]];
const names=['south-east','north-west','south','north','north-east','south-west'];
const valid=edges=>[...new Set((edges||[]).filter(i=>Number.isInteger(i)&&i>=0&&i<6))];
const directions=edges=>valid(edges).map(i=>names[i]).join(' and ');
export function riverFlowLines(tile,tiles=[]) {
 if(!['river','river_ford'].includes(tile?.terrain))return [];
 const lines=[];
 if(tile.river_flow_known) {
  const from=directions(tile.river_upstream_edges),to=directions(tile.river_downstream_edges);
  if(to)lines.push((from?'Flows from '+from:'Flows')+' toward '+to+'.');
 } else if(tile.river_flow_known===false)lines.push('Flow direction unavailable.');
 const fog=valid(tile.river_edges).filter(i=>{const [dq,dr]=RIVER_OFFSETS[i];const n=tiles.find(t=>t.q===tile.q+dq&&t.r===tile.r+dr);return !n||n.terrain==='fog';});
 if(fog.length)lines.push('Continues into the fog toward '+directions(fog)+'.');
 const kinds={river:'river',river_ford:'ford',coastal_sea:'coastal water',deep_sea:'deep sea'};
 const connections=(tile.river_connections||[]).filter(c=>valid(tile.river_edges).includes(c.edge)&&kinds[c.terrain]);
 if(connections.length)lines.push('Connections: '+connections.map(c=>names[c.edge]+' — '+kinds[c.terrain]).join('; ')+'.');
 if(Array.isArray(tile.river_edges)&&tile.river_edges.length===0)lines.push('The river ends here.');

 return lines;
}
function pixelLine(ctx,x0,y0,x1,y1) {
 x0=Math.round(x0);y0=Math.round(y0);x1=Math.round(x1);y1=Math.round(y1);
 const dx=Math.abs(x1-x0),sx=x0<x1?1:-1,dy=-Math.abs(y1-y0),sy=y0<y1?1:-1;let err=dx+dy;
 for(;;){ctx.fillRect(x0,y0,1,1);if(x0===x1&&y0===y1)break;const e=2*err;if(e>=dy){err+=dy;x0+=sx;}if(e<=dx){err+=dx;y0+=sy;}}
}
// Integer pixels only. Variants are exported for the BILD comparison artifact.
export function drawRiverFlow(ctx,cx,cy,tile,colour,variant='chevron') {
 if(!['river','river_ford'].includes(tile?.terrain)||!tile.river_flow_known||variant==='text')return;
 ctx.save();ctx.fillStyle=colour;
 for(const i of valid(tile.river_downstream_edges)) {
  const [dq,dr]=RIVER_OFFSETS[i];let nx=1.5*dq,ny=Math.sqrt(3)*(dr+dq/2);const len=Math.hypot(nx,ny);nx/=len;ny/=len;
  const draw=distance=>{const x=cx+nx*distance,y=cy+ny*distance;pixelLine(ctx,x-nx*3-ny*2,y-ny*3+nx*2,x,y);pixelLine(ctx,x,y,x-nx*3+ny*2,y-ny*3-nx*2);};
  draw(6);if(variant==='double')draw(1);
 }
 ctx.restore();
}
