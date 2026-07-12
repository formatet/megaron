// ── Sprint 10: Animated pixel city scene ─────────────────────────────────
// Canvas renderer; exempt from CSS vars (own palette).

const _CITY_BLD_PRIORITY = [
  'wall','bronze_wall','tower',
  'barracks','stable','harbour','foundry',
  'market','farm','mine','lumbermill','stonequarry',
  'winery','olive_press',
];
const _CITY_STRUCTURAL = new Set(['wall','bronze_wall','tower']);

let _cityAnimId = null;
export function stopCityAnim() {
  if (_cityAnimId !== null) { cancelAnimationFrame(_cityAnimId); _cityAnimId = null; }
}

export function startCityAnim(canvas, tile, buildings, buildQueue) {
  stopCityAnim();
  if (!canvas) return;
  const W = canvas.width, H = canvas.height;
  const S = 3, ground = H - 8;
  const ctx = canvas.getContext('2d');
  ctx.imageSmoothingEnabled = false;

  const wr = {x: 60, dir: 1, frame: 0, t: 0};
  const smoke = [];
  let last = 0;

  const hasfoundry = (buildings||[]).some(b => b.type === 'foundry');
  const smokeX1 = Math.floor(W/2) + 5*S;
  const smokeX2 = hasfoundry ? 218 : null;

  function tick(ts) {
    const dt = last ? Math.min((ts - last)/1000, 0.1) : 0.016;
    last = ts;
    wr.t += dt;
    if (wr.t > 0.26) { wr.frame ^= 1; wr.t = 0; }
    wr.x += wr.dir * 20 * S * dt;
    if (wr.x > W - 28 || wr.x < 18) wr.dir = -wr.dir;
    for (let i = smoke.length - 1; i >= 0; i--) {
      smoke[i].y -= dt * 8; smoke[i].age += dt;
      if (smoke[i].age > 1.4) smoke.splice(i, 1);
    }
    if (Math.random() < dt*2 && smoke.length < 6)
      smoke.push({x: smokeX1 + (Math.random()*2-1)*S, y: ground - 18*S, age: 0});
    if (smokeX2 && Math.random() < dt*2.5 && smoke.length < 8)
      smoke.push({x: smokeX2 + (Math.random()*2-1)*S, y: ground - 13*S, age: 0});
    _renderCityFrame(ctx, W, H, S, ground, tile, buildings, buildQueue, wr, smoke);
    _cityAnimId = requestAnimationFrame(tick);
  }
  _cityAnimId = requestAnimationFrame(tick);
}

function _renderCityFrame(ctx, W, H, S, ground, tile, buildings, buildQueue, wr, smoke) {
  const OUTLINE = '#2a1a08';
  const BG = {plains:'#c0b882', river_valley:'#a0c070', river_delta:'#8ccc70', hills:'#b0a268',
              mountain:'#989690', forest:'#728a60', coast:'#6aa0b0',
              coast_beach:'#c8b87a', sea:'#4870a0'};
  ctx.fillStyle = BG[tile?.terrain] || BG.plains;
  ctx.fillRect(0, 0, W, H);

  const bldSet = new Set((buildings||[]).map(b => b.type));
  const mx = Math.floor(W/2 - 48), mw = 96, mh = 14*S;

  // Wall background
  if (bldSet.has('wall') || bldSet.has('bronze_wall')) {
    const isBronze = bldSet.has('bronze_wall');
    const wallY = ground - 19*S;
    ctx.fillStyle = isBronze ? '#c8a040' : '#9a9080';
    ctx.fillRect(0, wallY, W, 3*S);
    ctx.fillStyle = isBronze ? '#8a6010' : '#706860';
    for (let cx = 4*S; cx < W - 3*S; cx += 5*S) ctx.fillRect(cx, wallY - 2*S, 3*S, 2*S);
    ctx.strokeStyle = OUTLINE; ctx.lineWidth = 1;
    ctx.strokeRect(0, wallY, W, 3*S);
  }

  // Tower (back-left, if built)
  if (bldSet.has('tower')) {
    const tx = 6*S, ty = ground - 14*S, tw = 5*S, th = 14*S;
    ctx.fillStyle = '#8a7870';
    ctx.fillRect(tx, ty, tw, th);
    ctx.strokeStyle = OUTLINE; ctx.lineWidth = 1; ctx.strokeRect(tx, ty, tw, th);
    ctx.fillStyle = '#585048';
    for (let ci = 0; ci < 3; ci++) ctx.fillRect(tx + ci*2*S, ty - S, S, S);
  }

  // Ground strip
  ctx.fillStyle = '#80702a'; ctx.fillRect(0, ground, W, H - ground);
  ctx.fillStyle = '#6a5c22';
  [18, 52, 100, 160, 200, 258, 292].forEach(gx => ctx.fillRect(gx, ground+2, S, S));

  // Megaron (center)
  const my = ground - mh;
  ctx.fillStyle = '#5a4820'; ctx.fillRect(mx - S, ground, mw + 2*S, S); // shadow
  ctx.fillStyle = '#d4b87a'; ctx.fillRect(mx, my, mw, mh);
  // Window slits
  ctx.fillStyle = OUTLINE;
  ctx.fillRect(mx + 5*S, my + 3*S, 2*S, 3*S);
  ctx.fillRect(mx + 25*S, my + 3*S, 2*S, 3*S);
  ctx.fillRect(mx + 13*S, my + 6*S, 6*S, mh - 6*S); // door
  // Columns
  ctx.fillStyle = '#f0e0a8';
  [3,9,20,26].forEach(col => {
    ctx.fillRect(mx + col*S, my, 2*S, mh - 2*S);
    ctx.strokeStyle = '#c8a870'; ctx.lineWidth = 1; ctx.strokeRect(mx + col*S, my, 2*S, mh - 2*S);
  });
  // Roof
  ctx.fillStyle = '#8a5c2a'; ctx.fillRect(mx - 2*S, my - 3*S, mw + 4*S, 3*S);
  ctx.fillStyle = '#5a3810'; ctx.fillRect(mx + 8*S, my - 4*S, 16*S, S); // ridge
  // Hearth chimney
  ctx.fillStyle = '#4a3828'; ctx.fillRect(mx + 14*S, my - 5*S, 4*S, 2*S);
  ctx.strokeStyle = OUTLINE; ctx.lineWidth = 1;
  ctx.strokeRect(mx, my, mw, mh);
  ctx.strokeRect(mx - 2*S, my - 3*S, mw + 4*S, 3*S);

  // Side building slots: 2 left, 2 right
  const BW = 12*S, BH = 10*S;
  const SLOTS = [
    {x: 6*S}, {x: mx - BW - 4*S},
    {x: mx + mw + 4*S}, {x: mx + mw + 4*S + BW + 4*S},
  ];
  const sideBlds = (buildings||[])
    .map(b => b.type).filter(t => !_CITY_STRUCTURAL.has(t))
    .sort((a,b) => _CITY_BLD_PRIORITY.indexOf(a) - _CITY_BLD_PRIORITY.indexOf(b))
    .slice(0, 4);
  sideBlds.forEach((type, i) => {
    if (i >= SLOTS.length) return;
    _drawSideBuilding(ctx, type, SLOTS[i].x, ground - BH, BW, BH, S);
  });

  // Construction scaffolds
  const qItems = (buildQueue||[]).slice(0, 4 - sideBlds.length);
  qItems.forEach((item, i) => {
    const si = sideBlds.length + i;
    if (si >= SLOTS.length) return;
    const phase = _cityPhase(item);
    _drawScaffold(ctx, SLOTS[si].x, ground - BH, BW, BH, phase, S);
  });

  // Walking worker
  _drawWorker(ctx, Math.round(wr.x), ground - 5*S, wr.frame, wr.dir, S);

  // Smoke particles
  for (const p of smoke) {
    const a = Math.max(0, (1 - p.age/1.4) * 0.55).toFixed(2);
    const r = Math.max(1, Math.floor(2 + p.age*2.5));
    ctx.fillStyle = `rgba(160,140,125,${a})`;
    ctx.fillRect(Math.round(p.x) - r, Math.round(p.y) - r, r*2, r*2);
  }
}

function _cityPhase(item) {
  const now = Date.now();
  const start = item.created_at ? new Date(item.created_at).getTime() : now - 30000;
  const end   = new Date(item.complete_at).getTime();
  if (end <= now) return 1;
  if (start >= end) return 0.5;
  return Math.min(1, Math.max(0, (now - start)/(end - start)));
}

function _drawSideBuilding(ctx, type, bx, by, bw, bh, S) {
  const OUTLINE = '#2a1a08';
  const COLORS = {
    barracks:    ['#7a8898','#4a5860'],
    farm:        ['#c8a850','#7a5c1a'],
    market:      ['#d09060','#8a4820'],
    harbour:     ['#5a7890','#2a4858'],
    foundry:     ['#504038','#282020'],
    mine:        ['#787068','#4a4038'],
    lumbermill:  ['#8a6040','#5a3820'],
    stable:      ['#b09060','#7a5828'],
    winery:      ['#7a4a6a','#4a2840'],
    olive_press: ['#8a8050','#4a4828'],
    stonequarry: ['#9a9088','#686058'],
  };
  const [wall, roof] = COLORS[type] || ['#a09080','#706860'];

  ctx.fillStyle = wall; ctx.fillRect(bx, by, bw, bh);
  ctx.fillStyle = roof; ctx.fillRect(bx - S, by - 2*S, bw + 2*S, 2*S);
  ctx.strokeStyle = OUTLINE; ctx.lineWidth = 1;
  ctx.strokeRect(bx, by, bw, bh);
  ctx.strokeRect(bx - S, by - 2*S, bw + 2*S, 2*S);

  ctx.fillStyle = OUTLINE;
  switch (type) {
    case 'barracks':
      ctx.fillRect(bx + 4*S, by + bh - 4*S, 4*S, 4*S); // door
      ctx.fillStyle = '#2a3840';
      ctx.fillRect(bx + 2*S, by + S, S, bh - 2*S); // spear L
      ctx.fillRect(bx + 9*S, by + S, S, bh - 2*S); // spear R
      break;
    case 'farm':
      // Field rows in front
      [0,1,2].forEach(fi => {
        ctx.fillStyle = fi%2===0 ? '#80b840' : '#a89030';
        ctx.fillRect(bx + fi*4*S, by + bh, 4*S, 2*S);
      });
      break;
    case 'harbour':
      ctx.fillStyle = '#3a6070';
      for (let hy = by + 3*S; hy < by + bh - S; hy += 2*S)
        ctx.fillRect(bx + S, hy, bw - 2*S, S); // plank lines
      ctx.fillStyle = '#3a2810'; ctx.fillRect(bx + bw - 2*S, by + bh/2, 2*S, bh/2); // post
      break;
    case 'foundry':
      ctx.fillStyle = '#282020'; ctx.fillRect(bx + bw - 3*S, by - 4*S, 3*S, 4*S); // chimney
      ctx.fillStyle = '#d04810'; ctx.fillRect(bx + 3*S, by + bh - 3*S, 6*S, 3*S); // glow
      break;
    case 'mine':
      ctx.fillStyle = OUTLINE; ctx.fillRect(bx + 3*S, by + bh*0.4, 6*S, bh*0.6);
      ctx.fillStyle = '#181008'; ctx.fillRect(bx + 4*S, by + bh*0.5, 4*S, bh*0.5);
      break;
    case 'market':
      ctx.fillStyle = '#e0c898';
      [1,5,9].forEach(col => {
        ctx.fillRect(bx + col*S, by, S, bh);
        ctx.strokeStyle = OUTLINE; ctx.strokeRect(bx + col*S, by, S, bh);
      });
      break;
    case 'stable':
      ctx.fillStyle = OUTLINE; ctx.fillRect(bx + 4*S, by + bh - 3*S, 4*S, 3*S); // door
      ctx.fillStyle = '#7a5828';
      [1,8].forEach(vx => ctx.fillRect(bx + vx*S, by + 2*S, S, 2*S)); // vents
      break;
    case 'lumbermill':
      [0,1,2].forEach(li => {
        ctx.fillStyle = li%2===0 ? '#704820' : '#906030';
        ctx.fillRect(bx + li*4*S, by + bh, 3*S, 2*S); // log pile
      });
      ctx.fillStyle = '#c0c0c0'; ctx.fillRect(bx + 5*S, by + 3*S, 5*S, S); // blade
      break;
    case 'winery':
      ctx.fillStyle = '#5a2040';
      [1,7].forEach(ax => {
        ctx.fillRect(bx + ax*S, by + 2*S, 2*S, 4*S);
        ctx.fillRect(bx + ax*S + S, by + 5*S, S, 2*S); // neck
      });
      break;
    case 'stonequarry':
      ctx.fillStyle = '#b0a890';
      ctx.fillRect(bx, by + bh - 2*S, bw, 2*S);
      ctx.fillRect(bx + 2*S, by + bh - 4*S, bw - 4*S, 2*S);
      break;
    case 'olive_press':
      ctx.fillStyle = '#606030'; ctx.fillRect(bx + 2*S, by + bh - 4*S, 8*S, 4*S);
      ctx.fillStyle = '#808050'; ctx.fillRect(bx + 4*S, by + bh - 5*S, 4*S, S);
      break;
  }
}

function _drawScaffold(ctx, bx, by, bw, bh, phase, S) {
  const OUTLINE = '#2a1a08';
  if (phase > 0.6) {
    ctx.fillStyle = 'rgba(200,165,85,0.4)';
    ctx.fillRect(bx, by, bw, bh);
  }
  ctx.fillStyle = '#c89858';
  ctx.fillRect(bx - S, by, S, bh + 2*S);
  ctx.fillRect(bx + bw, by, S, bh + 2*S);
  if (phase > 0.3) ctx.fillRect(bx - S, by + Math.floor(bh*0.5), bw + 2*S, S);
  if (phase > 0.6) ctx.fillRect(bx - S, by, bw + 2*S, S);
  ctx.strokeStyle = '#7a5030'; ctx.lineWidth = 1;
  ctx.beginPath(); ctx.moveTo(bx, by); ctx.lineTo(bx + bw, by + bh); ctx.stroke();
  ctx.beginPath(); ctx.moveTo(bx + bw, by); ctx.lineTo(bx, by + bh); ctx.stroke();
  // Construction worker with hammer at scaffold base
  const wx = bx + Math.floor(bw/2);
  ctx.fillStyle = '#c8a060'; ctx.fillRect(wx - S, by + bh - 4*S, 2*S, 3*S);
  ctx.fillStyle = '#a07040'; ctx.fillRect(wx - S, by + bh - 5*S, 2*S, S);
  ctx.fillStyle = '#888'; ctx.fillRect(wx + S, by + bh - 4*S, S, 3*S);
  ctx.fillStyle = '#666'; ctx.fillRect(wx + S, by + bh - 4*S, 2*S, S);
}

function _drawWorker(ctx, wx, wy, frame, dir, S) {
  ctx.fillStyle = '#c8a070'; ctx.fillRect(wx, wy - 5*S, 2*S, 2*S); // head
  ctx.fillStyle = '#6a7080'; ctx.fillRect(wx, wy - 3*S, 2*S, 2*S); // body
  ctx.fillStyle = '#8a6840';
  if (frame === 0) {
    ctx.fillRect(wx, wy - S, S, S);
    ctx.fillRect(wx + S, wy - 2*S, S, S);
  } else {
    ctx.fillRect(wx + S, wy - S, S, S);
    ctx.fillRect(wx, wy - 2*S, S, S);
  }
}

