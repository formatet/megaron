// Order 25: the street climbs toward the sanctuary, between inhabited terraces.
// 21c supplies the geometry; 03 supplies the light mountain horizon. Population
// changes residential mass, while functions and statuses come from the API.
import { CITY_PALETTE, newGrid } from './pixelgrid.js';
import { stampBuilding, buildingWidth } from './citybuildings.js';

const W = 160, H = 96, S = 2;
export const SCENE_CANVAS = { w: W * S, h: H * S };
const P = {
  ink:'#1F1A14', light:'#EFE4CB', plaster:'#D2C3A0', shade:'#A08D71',
  roof:'#C0A473', edge:'#836A44', earth:'#C4A578', stone:'#B5A992',
  rock:'#6E6552', red:'#B0442A', blood:'#922B21', wood:'#6B5334',
  sea:'#827F8C', foam:'#ABA3A5', field:'#B5A062', sky:'#E3C6A0',
  far:'#B59B89', mountain:'#8C8273', band:'#C2A894', smoke:'#938F84',
};

// Integer software raster, shared by the browser and the structural tests.
// No canvas paths: polygons and Bresenham edges always land on whole pixels.
function painter(pixels, housing) {
  let residential = false;
  function pixel(x,y,c) {
    if(x>=0 && x<W && y>=0 && y<H) {
      const i=y*W+x; pixels[i]=P[c] || c;
      if(housing) housing[i]=residential ? 1 : 0;
    }
  }
  function rect(x1,y1,x2,y2,c) {
    for(let y=Math.max(0,y1);y<=Math.min(H-1,y2);y++)
      for(let x=Math.max(0,x1);x<=Math.min(W-1,x2);x++) pixel(x,y,c);
  }
  function line(points,c) {
    for(let i=1;i<points.length;i++) {
      let [x,y]=points[i-1];const [xx,yy]=points[i];
      const dx=Math.abs(xx-x),dy=-Math.abs(yy-y),sx=x<xx?1:-1,sy=y<yy?1:-1;
      let err=dx+dy;
      for(;;) {pixel(x,y,c);if(x===xx && y===yy)break;const e=2*err;
        if(e>=dy){err+=dy;x+=sx;}if(e<=dx){err+=dx;y+=sy;}}
    }
  }
  function poly(points,c) {
    const ymin=Math.max(0,Math.min(...points.map(p=>p[1]))), ymax=Math.min(H-1,Math.max(...points.map(p=>p[1])));
    for(let y=ymin;y<=ymax;y++) {
      const xs=[];
      for(let i=0,j=points.length-1;i<points.length;j=i++) {
        const [x1,y1]=points[j],[x2,y2]=points[i];
        if((y1<=y && y2>y)||(y2<=y && y1>y))xs.push(x1+(y-y1)*(x2-x1)/(y2-y1));
      }
      xs.sort((a,b)=>a-b);
      for(let i=0;i+1<xs.length;i+=2)rect(Math.ceil(xs[i]),y,Math.floor(xs[i+1]),y,c);
    }
    line([...points,points[0]],c);
  }
  function box(x,y,w,h,d=3,reverse=false,temple=false) {
    if(reverse) {
      poly([[x,y],[x+w,y],[x+w-d,y-d],[x-d,y-d]],'roof');
      rect(x,y+1,x+w,y+h,'shade');
      poly([[x-d,y-d],[x,y],[x,y+h],[x-d,y+h-d]],'plaster');
      line([[x-d,y-d],[x+w-d,y-d]],'light');
    } else {
      poly([[x,y],[x+w,y],[x+w+d,y-d],[x+d,y-d]],temple?'light':'roof');
      rect(x,y+1,x+w,y+h,temple?'plaster':'shade');
      poly([[x+w,y],[x+w+d,y-d],[x+w+d,y+h-d],[x+w,y+h]],temple?'shade':'edge');
      line([[x,y],[x+w,y]],temple?'light':'earth');
    }
    rect(x+Math.floor(w/2),y+h-3,x+Math.floor(w/2)+1,y+h,'ink');
  }
  return {pixel,rect,line,poly,box,mass(on){residential=on;}};
}

const OTHER_FUNCTIONS = ['olive_press','winery','market','lumbermill','stonequarry','silver_mine','foundry','barracks','stable'];
const array = () => Array(W*H).fill(null);

/** Pure snapshot of exactly the payload used by the city drawer.
 * structure excludes status/life; housing is the visible residential projection
 * after occlusion, not a count of houses or a measure of human recognition. */
export function buildCityScene(tile, buildings=[], buildQueue=[], sett={}) {
  buildings ||= [];buildQueue ||= [];sett ||= {};
  const structure=Array(W*H).fill(P.sky),housing=new Uint8Array(W*H);
  const status=array(),life=array(),d=painter(structure,housing);
  const {rect,line,poly,box}=d;
  const tier=sett.population>=8000?2:sett.population>=3000?1:0;
  const terraceLevels=[1,3,5][tier];
  const walls=Math.max(0,Math.min(3,Number(sett.walls)||0));
  const starving=sett.food?.growth_state==='shrinking',besieged=sett.besieged===true;
  const byType=new Map();
  for(const b of buildings)if(b.level>0)byType.set(b.type,Math.max(byType.get(b.type)||0,b.level));
  const anchors={gate:[80,60,7,5]};
  poly([[0,30],[0,20],[13,14],[27,18],[42,11],[57,17],[78,10],[99,19],[125,13],[144,19],[160,15],[160,52],[0,52]],'far');
  // Warm, light bands: darker 21b roof/rock substitutions are deliberately absent.
  poly([[0,23],[13,17],[27,21],[42,14],[57,20],[78,13],[99,22],[125,16],[144,22],[160,18],[160,25],[140,26],[119,21],[102,26],[80,18],[57,25],[36,20],[14,24],[0,27]],'band');
  poly([[0,38],[23,29],[47,32],[69,24],[94,29],[118,23],[145,31],[160,27],[160,65],[0,65]],'mountain');
  poly([[0,58],[28,43],[55,36],[93,35],[125,44],[160,57],[160,96],[0,96]],'earth');
  poly([[46,96],[76,44],[89,43],[118,96]],'light');
  for(let y=59;y<94;y+=5){const half=Math.round((y-42)*.35)+5;line([[82-half,y],[82+half,y]],'shade');}

  function home(x,y,w,h,depth,reverse=false) {
    poly([[x+2,y+h],[x+w+depth+4,y+h-depth],[x+w+depth+6,y+h-depth+3],[x+5,y+h+4]],'rock');
    d.mass(true);box(x,y,w,h,depth,reverse);d.mass(false);
  }
  // Back to front: contiguous inhabited shelves, separated from the mountain.
  for(let row=terraceLevels-1;row>=0;row--) {
    const y=70-row*9,outer=17-row*2;
    poly([[outer,y+13],[68,y+5],[68,y+8],[outer,y+17]],'rock');
    poly([[97,y+5],[149-Math.floor(outer/3),y+12],[149-Math.floor(outer/3),y+16],[97,y+8]],'rock');
    if(tier===0) {home(22,66,19,14,4);home(122,63,19,17,4,true);}
    else for(const [x,w,dy,rev] of [[10+row*3,16,3,false],[36+row*2,17,-1,false],[108-row,16,0,true],[132-row*2,15,4,true]])home(x,y+dy,w,row<3?10:8,3,rev);
  }
  if(tier>0){home(0,79,30,23,5);home(132,76,26,24,5,true);}

  // Sanctuary only if built: a flat roof and red columns, never a generic
  // population-generated palace or a classical pediment.
  if(byType.has('temple')) {
    const level=byType.get('temple'),x=72,y=43,w=22,h=12;
    box(x,y,w,h,5,false,true);rect(x-2,y+h+1,x+w+3,y+h+2,'stone');
    rect(x+2,y+3,x+w-2,y+4,'red');
    for(const xx of [x+4,x+w-5]) {rect(xx,y+4,xx+2,y+h,'red');rect(xx+1,y+6,xx+2,y+h,'wood');}
    rect(x+8,y+5,x+w-8,y+h,'ink');
    for(let i=0;i<Math.min(3,level);i++)rect(x+2+i*4,y-3,x+4+i*4,y-1,'light');
    anchors.temple=[x,y,w,h];
  }
  // A single fortification across the ascent, with stepped level silhouettes.
  if(walls) {
    const foot=65,top=foot-[0,5,9,14][walls];
    rect(40,top,125,foot,'rock');line([[40,top],[125,top]],'stone');
    for(let y=top+3;y<foot;y+=3){line([[40,y],[125,y]],'shade');for(let x=40+(y%2)*4;x<125;x+=9)line([[x,y],[x,y+2]],'stone');}
    const gw=[0,3,4,5][walls];rect(80-gw,60,80+gw,65,'ink');
    line([[78-gw,65],[78-gw,58],[82+gw,58],[82+gw,65]],'stone');
    if(walls>=2)for(let x=40;x<125;x+=5)rect(x,top-2,x+2,top,'stone');
    if(walls===3)for(const x of [60,103]){rect(x,top-5,x+7,65,'rock');rect(x,top-5,x+6,top-3,'stone');rect(x+2,top-1,x+3,top+2,'ink');}
  }
  line([[108,46],[108,31]],'ink');poly([[109,31],[118,33],[115,36],[109,36]],'red');rect(111,33,112,34,'light');

  if(byType.has('farm')) {
    poly([[1,68],[32,64],[38,72],[6,76]],'field');
    for(let i=0;i<Math.min(6,byType.get('farm')+2);i++)line([[5,68+i*2],[32,64+i*2]],'edge');
    box(3,63,8,5,2);anchors.farm=[1,63,37,13];
    if(tier===0) {poly([[37,55],[63,51],[69,59],[42,63]],'field');for(let i=0;i<4;i++)line([[41,55+i*2],[63,51+i*2]],'edge');}
  }
  if(byType.has('mine')) {
    poly([[139,53],[142,42],[152,39],[160,53]],'stone');rect(147,45,152,52,'ink');
    line([[146,52],[146,44],[153,44],[153,52]],'wood');line([[149,53],[157,58]],'edge');anchors.mine=[139,39,20,19];
  }
  // Sea contact comes from a real harbour/shipyard (these cannot be built inland).
  // A harbour does not fabricate a ship in the settlement's garrison.
  if(byType.has('harbour')||byType.has('shipyard')) {
    poly([[0,81],[21,74],[37,81],[0,96]],'sea');line([[0,91],[14,91]],'foam');
    if(byType.has('harbour')) {poly([[0,82],[30,82],[34,86],[0,86]],'stone');rect(8,85,12,95,'wood');line([[8,86],[8,95]],'light');anchors.harbour=[0,82,34,13];}
    if(byType.has('shipyard')) {line([[0,79],[0,86],[8,89],[16,89]],'wood');line([[0,78],[14,83]],'wood');for(let i=0;i<3;i++)line([[5+i*4,83],[8+i*4,87]],'ink');anchors.shipyard=[0,78,17,12];}
  }
  // Other functional silhouettes use the existing building family, in stable
  // side-street slots. The landscape/camera is unchanged by payload ordering.
  let slot=0;
  for(const type of OTHER_FUNCTIONS)if(byType.has(type)) {
    const level=byType.get(type),width=buildingWidth(type,level);
    const x=slot%2?Math.max(100,158-width):2,y=49+Math.floor(slot/2)*10;
    const g=newGrid(W,H);stampBuilding(g,type,x,y,level);
    for(let yy=0;yy<H;yy++)for(let xx=0;xx<W;xx++)if(g.px[yy*W+xx]!=='.')d.pixel(xx,yy,CITY_PALETTE[g.px[yy*W+xx]]);
    anchors[type]=[x,y-16,width,17];slot++;
  }

  const construction=[];
  const sd=painter(status);
  for(const item of [...buildQueue].sort((a,b)=>String(a.type).localeCompare(String(b.type)))) {
    if(!item.type)continue;
    const index=construction.length;
    const anchor=(['wall','bronze_wall','tower'].includes(item.type)?[40,51,85,14]:anchors[item.type])||(item.type==='temple'?[72,43,22,12]:[index%2?114:12,72+Math.floor(index/2)*8,24,13]);
    construction.push({type:item.type,anchor});
    const [x,y,w,h]=anchor;
    // Scaffold at the actual queued function, including first construction.
    for(const xx of [x-3,x+Math.floor(w/2),x+w+3])sd.line([[xx,y-6],[xx,y+h+3]],'wood');
    for(const yy of [y-4,y+3,y+h])sd.line([[x-4,yy],[x+w+4,yy]],'wood');
    sd.line([[x-3,y+h],[x+w+3,y-4]],'wood');sd.rect(x+w+5,y+h+1,x+w+11,y+h+3,'stone');
  }
  if(besieged) {
    // Known siege truth, no invented enemy camp coordinates or army counts.
    sd.rect(77,60,83,65,'wood');sd.line([[77,60],[83,65]],'ink');
    for(const [x,y] of [[32,58],[39,57]]){sd.line([[x,y],[x,y-7]],'ink');sd.poly([[x-1,y-6],[x,y-9],[x+1,y-6]],'stone');}
  }
  function bowl(q,x,y) {q.poly([[x-5,y-2],[x+5,y-2],[x+3,y+2],[x-3,y+2]],'wood');q.line([[x-3,y-1],[x+3,y-1]],'ink');}
  if(starving) {
    sd.rect(74,79,76,81,'wood');sd.line([[74,82],[72,86],[76,87]],'shade');sd.line([[76,87],[81,88]],'rock');
    bowl(sd,60,89);bowl(sd,88,90);
    sd.rect(4,78,26,93,'light');sd.line([[5,79],[25,79]],'blood');bowl(sd,16,87);sd.line([[16,82],[16,80]],'blood');
  }
  const ld=painter(life);
  const folk=starving?[]:[[67,83,2],[105,87,2],[83,69,1],[51,90,1],[119,76,1],[76,77,1]].slice(0,[2,4,6][tier]);
  for(const [x,y,s] of folk){ld.rect(x,y-4*s,x+s-1,y-3*s,'wood');ld.rect(x-s,y-2*s,x+s,y,'red');ld.line([[x-s,y+1],[x-s,y+2*s]],'ink');ld.line([[x+s,y+1],[x+s,y+2*s]],'ink');}
  // Residential hearths represent inhabited housing in population bands.
  // temple_offers.fed describes the NEXT gift, and devotion is staffing;
  // neither proves a sacrifice occurred, so no sacrificial smoke is invented.
  const vents=starving?[]:[[22,63],[132,60],[40,47]].slice(0,[1,2,3][tier]);
  return {structure,housing,status,life,terraceLevels,anchors,starving,besieged,construction,vents};
}

function paint(ctx,pixels) {
  for(let y=0;y<H;y++)for(let x=0;x<W;) {
    const c=pixels[y*W+x];let end=x+1;while(end<W && pixels[y*W+end]===c)end++;
    if(c){ctx.fillStyle=c;ctx.fillRect(x*S,y*S,(end-x)*S,S);}x=end;
  }
}

/** Same integer painter for animation and the reproducible BILD gallery. */
export function drawCityFrame(ctx,scene,phase=0) {
  paint(ctx,scene.structure);paint(ctx,scene.life);paint(ctx,scene.status);
  for(const [x,y] of scene.vents) {
    const smoke=array(),d=painter(smoke);
    d.line([[x,y],[x-1,y-3],[x+1,y-6-phase]],'smoke');paint(ctx,smoke);
  }
}

let animId=null;
export function stopCityAnim(){if(animId!==null){cancelAnimationFrame(animId);animId=null;}}
export function startCityAnim(canvas,tile,buildings,buildQueue,sett) {
  stopCityAnim();if(!canvas)return;
  canvas.width=SCENE_CANVAS.w;canvas.height=SCENE_CANVAS.h;
  const ctx=canvas.getContext('2d');ctx.imageSmoothingEnabled=false;
  const scene=buildCityScene(tile,buildings,buildQueue,sett);
  let epoch=null;
  drawCityFrame(ctx,scene);
  function frame(ts) {
    if(epoch===null)epoch=ts;
    drawCityFrame(ctx,scene,Math.floor((ts-epoch)/400)%6);
    animId=requestAnimationFrame(frame);
  }
  animId=requestAnimationFrame(frame);
}
