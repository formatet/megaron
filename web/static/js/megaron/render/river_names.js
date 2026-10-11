// Names arrive from Temenos. Never derive chains from a partial FOW view.
const riverFamily = new Set(['river', 'river_ford', 'river_delta']);
export function namedRiverTerrain(tile, fallback) {
  if (!tile.river_name || tile.terrain === 'fog' || tile.tier === 'fog' || tile.visible === false) return fallback;
  switch (tile.terrain) {
    case 'river': return `River ${tile.river_name}`;
    case 'river_ford': return `Ford of ${tile.river_name}`;
    case 'river_delta': return `Delta of ${tile.river_name}`;
    default: return fallback;
  }
}
export const riverLabelOverlap = (a,b) => a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;

// Logical world-pixel rectangles include the text's stroke and clearance.
// City labels use the renderer's existing 7/6 px fonts and cy+19 anchor.
export function riverLabelLayout(tiles, provinces, { hexPx, measure, zoom, viewport }) {
  if (zoom < .55) return [];
  const occupied = [];
  for (const p of provinces) {
    const {x,y} = hexPx(p.q,p.r), w = measure(p.name,!!p.own)+4;
    occupied.push({left:x-w/2,right:x+w/2,top:y+17,bottom:y+29});
    occupied.push({left:x-24,right:x+24,top:y-30,bottom:y+18});
  }
  const groups = new Map();
  for (const t of tiles) {
    if (!t.river_name || !riverFamily.has(t.terrain) || t.terrain === 'fog' || t.tier === 'fog' || t.visible === false) continue;
    const {x,y} = hexPx(t.q,t.r);
    if (x < viewport.left-25 || x > viewport.right+25 || y < viewport.top-25 || y > viewport.bottom+25) continue;
    if (!groups.has(t.river_name)) groups.set(t.river_name,[]);
    groups.get(t.river_name).push(t);
  }
  const out = [];
  for (const [name, group] of [...groups].sort(([a],[b]) => a.localeCompare(b))) {
    const cx = group.reduce((n,t)=>n+hexPx(t.q,t.r).x,0)/group.length;
    const cy = group.reduce((n,t)=>n+hexPx(t.q,t.r).y,0)/group.length;
    group.sort((a,b)=>{
      const p=hexPx(a.q,a.r),q=hexPx(b.q,b.r);
      return Math.hypot(p.x-cx,p.y-cy)-Math.hypot(q.x-cx,q.y-cy) || a.q-b.q || a.r-b.r;
    });
    const w=measure(name,false)+4;
    let chosen;
    for (const t of group) {
      const {x,y}=hexPx(t.q,t.r);
      for (const dy of [-14,9,-25,20]) {
        const rect={left:x-w/2,right:x+w/2,top:y+dy-2,bottom:y+dy+10};
        if (rect.left < viewport.left || rect.right > viewport.right || rect.top < viewport.top || rect.bottom > viewport.bottom) continue;
        if (occupied.some(r=>riverLabelOverlap(rect,r))) continue;
        chosen={name,x,y:y+dy,rect};break;
      }
      if (chosen) break;
    }
    if (chosen) { out.push(chosen);occupied.push(chosen.rect); }
  }
  return out;
}
