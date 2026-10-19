#!/usr/bin/env python3
"""Parameterized 160x96 city scenes. No edits to production assets or earlier studies."""
import importlib.util
import json
import hashlib
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont, ImageOps

OUT=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('city_study_base',OUT.parent/'generate.py')
base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)
STATES=base.STATES
LABELS=base.LABELS
W,H=160,96
POPS=[1000,4500,10000]
WALLS=[0,1,3]
VARIANTS=['huvud','djup','varma-berg']
TITLES={'gata':'04 · Gatan mot helgedomen','tak':'10 · Över taken mot gården'}

class City(base.Scene):
    def __init__(self,population=4500,walls=1,variant='huvud'):
        palette={}
        if variant=='varma-berg':
            palette={'sky':'#E3C6A0','far':'#B59B89','olive':'#8C8273','shade':'#896858','earth':'#B88C64','roof':'#BC9B75','rock':'#68565A','sea':'#827F8C','foam':'#ABA3A5'}
        super().__init__((W,H),palette)
        self.population=population;self.walls=walls;self.variant=variant
        self.tier=0 if population<3000 else 1 if population<8000 else 2
        self.folk=[];self.vents=[];self.camp=[];self.features={}
    def mountains(self):
        # The horizon remains visible between buildings at every data scale.
        self.poly([(0,30),(0,20),(13,14),(27,18),(42,11),(57,17),(78,10),(99,19),(125,13),(144,19),(160,15),(160,52),(0,52)],'far')
        self.poly([(0,38),(23,29),(47,32),(69,24),(94,29),(118,23),(145,31),(160,27),(160,65),(0,65)],'olive')
        if self.variant=='varma-berg':
            self.poly([(0,21),(13,15),(27,19),(42,12),(57,18),(78,11),(99,20),(125,14),(144,20),(160,16),(160,24),(140,25),(119,20),(102,25),(80,17),(57,24),(36,19),(14,23),(0,26)],'shade')
    def fortification(self,x1,x2,foot,gate):
        self.anchors['gate']=(gate,foot-4,5)
        if self.walls==0:return
        height={1:5,2:9,3:14}[self.walls];top=foot-height
        self.rect((x1,top,x2,foot),'rock');self.line([(x1,top),(x2,top)],'stone',2)
        for y in range(top+3,foot,3):
            self.line([(x1,y),(x2,y)],'shade')
            for x in range(x1+(y%2)*4,x2,9):self.line([(x,y),(x,y+2)],'stone')
        gatew={1:3,2:4,3:5}[self.walls]
        self.rect((gate-gatew,foot-5,gate+gatew,foot),'ink')
        self.line([(gate-gatew-2,foot),(gate-gatew-2,foot-7),(gate+gatew+2,foot-7),(gate+gatew+2,foot)],'stone',2)
        if self.walls>=2:
            for x in range(x1,x2,5):self.rect((x,top-2,x+2,top),'stone')
        if self.walls==3:
            for x in [x1+20,x2-22]:
                self.rect((x,top-5,x+7,foot),'rock');self.rect((x,top-5,x+6,top-3),'stone')
                self.rect((x+2,top-1,x+3,top+2),'ink')
        self.anchors['gate']=(gate,foot-5,5)
    def bowl(self,x,y,empty=False):
        self.poly([(x-5,y-2),(x+5,y-2),(x+3,y+2),(x-3,y+2)],'wood','ink')
        self.line([(x-3,y-1),(x+3,y-1)],'ink' if empty else 'field')
    def status_layer(self,state):
        structure=self.im.copy();layer=Image.new('RGBA',(W,H));self.im=layer;self.d=ImageDraw.Draw(layer)
        if state!='starvation':
            for x,y,size in self.folk:self.person(x,y,size)
            for x,y in self.vents:self.line([(x,y),(x-1,y-3),(x+1,y-6)],'smoke')
            self.bowl(*self.features['meal_point'])
            if self.features.get('foreground_wall'):
                self.d.rectangle(self.features['foreground_wall'],fill=(0,0,0,0))
        else:
            # Silenced street, closed shutters and visibly empty serving vessels.
            for x1,y1,x2,y2 in self.features['shutters']:
                self.rect((x1,y1,x2,y2),'wood');self.line([(x1,y1),(x2,y2)],'edge')
            x,y=self.features['hunger_point']
            self.rect((x,y-8,x+2,y-6),'wood')
            self.line([(x,y-5),(x-2,y-1),(x+2,y)],'shade',2)
            self.line([(x+2,y),(x+7,y+1)],'rock')
            for x,y in self.features['empty_bowls']:self.bowl(x,y,True)
            # Explicit separate status symbol: an empty bowl, visible even without a comparison.
            self.rect((4,78,26,93),'light')
            self.line([(5,79),(25,79)],'blood')
            self.poly([(8,84),(22,84),(19,89),(11,89)],'wood','ink')
            self.line([(10,84),(20,84)],'ink',2)
            self.line([(16,82),(16,80)],'blood')
        if state=='siege':
            x,y,hh=self.anchors['gate']
            self.rect((x-3,y,x+3,y+hh),'wood');self.line([(x-3,y),(x+3,y+hh)],'ink')
            # Camps are seen through a side opening beyond the defensive line.
            for x,y in self.camp:
                self.poly([(x-5,y+4),(x,y-3),(x+6,y+4)],'blood','ink')
                self.line([(x,y-3),(x,y+4)],'light')
                self.rect((x+7,y+2,x+9,y+3),'fire')
                self.line([(x+8,y+1),(x+7,y-2),(x+8,y-5)],'smoke')
            # Red spearheads in the outer distance supply a second, physical threat cue.
            for x,y in [(32,58),(39,57)]:
                self.line([(x,y),(x,y-7)],'ink');self.poly([(x-1,y-6),(x,y-9),(x+1,y-6)],'stone')
        if state=='construction':
            x,y,ww,hh=self.anchors['temple']
            for xx in [x-3,x+ww//2,x+ww+3]:self.line([(xx,y-6),(xx,y+hh+4)],'wood')
            for yy in [y-4,y+3,y+hh]:self.line([(x-4,yy),(x+ww+4,yy)],'wood')
            self.line([(x-3,y+hh),(x+ww+3,y-4)],'wood')
            self.rect((x+ww+5,y+hh+1,x+ww+12,y+hh+4),'stone')
            self.line([(x+ww+4,y-3),(x+ww+11,y+hh+3)],'wood')
            for i in range(4):self.line([(x+ww+5+i,y+i*3),(x+ww+8+i,y+i*3)],'wood')
        self.im=Image.alpha_composite(structure.convert('RGBA'),layer).convert('RGB');self.d=ImageDraw.Draw(self.im)
        return layer

def street(population=4500,walls=1,variant='huvud'):
    s=City(population,walls,variant);s.mountains()
    vp=88 if variant=='djup' else 80
    s.poly([(0,96),(vp-14,48),(vp+18,48),(160,96)],'earth')
    s.farm(10,37,22);s.mine(139,30)
    # The city behind the street grows in bounded population bands, not a building per citizen.
    plots=[(46,40,10,8,3),(102,39,11,9,3),(57,35,10,8,3),(115,41,11,10,3),
           (34,43,10,9,3),(124,39,10,11,3),(48,32,10,9,3),(107,31,11,10,3),
           (25,41,9,10,3),(132,43,9,10,3),(37,30,9,8,3),(119,29,10,8,3)]
    for x,y,w,h,d in plots[:[3,7,12][s.tier]]:s.box(x,y,w,h,d)
    s.temple(vp-11,36,22,12);s.flag(vp+17,23)
    s.fortification(42,125,62,vp)
    # Foreground houses form perspective planes and scale the width of the street.
    spread=[28,39,49][s.tier]+(3 if variant=='djup' else 0)
    farleft=vp-25;farright=vp+26
    lefttop=[50,31,22][s.tier];righttop=[43,23,14][s.tier]
    s.poly([(0,lefttop+5),(farleft,46),(farleft,61),(spread,96),(0,96)],'plaster')
    s.poly([(0,lefttop),(farleft+2,42),(farleft+2,46),(0,lefttop+5)],'roof')
    s.poly([(farright,44),(160,righttop+5),(160,96),(160-spread,96),(farright,61)],'edge')
    s.poly([(farright,40),(160,righttop),(160,righttop+5),(farright,45)],'roof')
    s.poly([(0,lefttop+6),(8,lefttop+7),(8,95),(0,96)],'shade')
    s.poly([(132,54),(144,49),(144,88),(132,80)],'ink')
    wy=max(lefttop+10,50)
    s.rect((18,wy,22,wy+11),'wood');s.rect((35,wy,38,wy+7),'wood')
    if s.tier==2:
        s.rect((18,36,22,43),'wood');s.rect((35,39,38,45),'wood')
        s.line([(0,46),(37,51)],'shade')
    s.line([(farleft,61),(spread,96)],'light')
    s.line([(farright,61),(160-spread,96)],'wood')
    # A small left-side coastal aperture keeps harbour/shipyard tied to the sea.
    s.water([(0,77),(25,69),(42,75),(0,91)]);s.harbour(1,77)
    s.line([(vp-7,66),(vp-23,96)],'shade');s.line([(vp+8,65),(vp+29,96)],'shade')
    folk=[(68,82,2),(105,86,2),(vp+3,66,1),(50,91,1),(118,72,1),(vp-8,75,1)]
    if variant=='djup':folk=[(56,91,2),(115,91,2),(vp-2,66,1),(vp+3,75,1),(112,76,1),(67,79,1)]
    s.folk=folk[:[2,4,6][s.tier]];s.vents=[(22,30),(145,24)]
    s.camp=[(33,56),(126,56)];s.features['shutters']=[(18,wy,22,wy+11),(132,54,140,78)]
    s.features['housing_masses']=[3,7,12][s.tier]+2
    s.features['people']=[2,4,6][s.tier]
    s.features['meal_point']=(58,89);s.features['hunger_point']=(74,87);s.features['empty_bowls']=[(60,89),(88,90)]
    return s

def roofs(population=4500,walls=1,variant='huvud'):
    s=City(population,walls,variant);s.mountains()
    s.poly([(0,59),(42,40),(83,30),(120,39),(160,64),(160,96),(0,96)],'earth')
    s.farm(1,44,25);s.mine(143,30)
    plots=[(43,44,17,12,5),(103,44,18,12,5),(24,49,14,13,4),(122,48,14,12,4),
           (48,35,13,10,4),(107,34,12,11,4),(9,52,12,10,3),(137,52,12,10,3),
           (34,32,11,9,3),(120,31,11,10,3),(46,59,10,10,3),(107,58,11,10,3)]
    for x,y,w,h,d in plots[:[3,7,12][s.tier]]:s.box(x,y,w,h,d)
    s.temple(70,31,24,13);s.flag(103,20)
    # The tall court opening preserves a horizon slit as density increases.
    s.poly([(62,54),(99,51),(124,90),(34,90)],'light')
    s.line([(63,55),(98,52)],'shade');s.line([(73,57),(66,79)],'shade')
    if variant=='djup':
        s.box(4,62,37,25,7);s.box(121,57,33,28,7)
        s.box(31,87,27,11,5);s.box(106,86,27,12,5)
    else:
        s.box(13,66,24+[0,3,6][s.tier],18,6);s.box(122,64,24,20,6)
        if s.tier>0:s.box(33,85,24,12,5);s.box(103,87,25,10,5)
    s.fortification(5,153,94,80)
    s.water([(0,95),(39,94),(120,95),(151,85),(160,85),(160,96),(0,96)]);s.harbour(126,86)
    s.folk=[(71,74,1),(94,63,1),(105,77,1),(58,84,1),(88,84,1),(113,87,1)][: [2,4,6][s.tier]]
    s.vents=[(23,58),(132,55)];s.camp=[(25,52),(132,43)]
    s.features['shutters']=[(24,75,28,84),(135,73,140,83)]
    s.features['housing_masses']=[5,11,16][s.tier]+(2 if variant=='djup' else 0)
    s.features['people']=[2,4,6][s.tier]
    s.features['meal_point']=(78,78);s.features['hunger_point']=(83,75);s.features['empty_bowls']=[(70,80),(96,81)]
    if walls:s.features['foreground_wall']=(5,94-{1:5,2:9,3:14}[walls]+1,153,94)
    return s

RENDER={'gata':street,'tak':roofs}

def save(im,stem):
    for scale in [1,2,4]:
        name=stem+('' if scale==1 else f'-{scale}x')+'.png'
        (im if scale==1 else im.resize((W*scale,H*scale),Image.Resampling.NEAREST)).save(OUT/name)

def card(row,game=True):
    stem=row['stem'];desc=f"{TITLES[row['camera']]} · {row['population']:,} inv · mur {row['walls']} · {LABELS[row['state']]} · {row['variant']}".replace(',',' ')
    return f'<figure><figcaption>{desc}</figcaption><div class="viewport"><img src="{stem}-2x.png" width="320" height="192" alt="{desc}"></div><details><summary>Logisk 1:1 och ×4</summary><div class="scroll"><img src="{stem}.png" width="160" height="96" alt="Logisk raster"><img src="{stem}-4x.png" width="640" height="384" alt="Fyra gångers raster"></div></details></figure>'

def main():
    rows=[]
    def render(camera,pop,walls,state,variant,group):
        stem=f'{camera}-p{pop}-m{walls}-{state}-{variant}'
        if not any(r['stem']==stem for r in rows):
            s=RENDER[camera](pop,walls,variant);structure=s.im.copy();layer=s.status_layer(state);save(s.im,stem)
            structure.save(OUT/(stem+'-structure.png'));layer.save(OUT/(stem+'-layer.png'))
            rows.append(dict(stem=stem,camera=camera,population=pop,walls=walls,state=state,variant=variant,groups=[group],features=s.features,anchors=s.anchors))
        else:
            next(r for r in rows if r['stem']==stem)['groups'].append(group)
    for camera in RENDER:
        for state in STATES:render(camera,4500,1,state,'huvud','status')
        for pop in POPS:
            for walls in WALLS:render(camera,pop,walls,'normal','huvud','scale')
        for variant in VARIANTS:render(camera,4500,1,'normal',variant,'variants')
    (OUT/'manifest.json').write_text(json.dumps(dict(size=[W,H],game_scale=2,reference=base.DATA,scenes=rows),ensure_ascii=False,indent=2)+'\n')
    intro='''<!doctype html><html lang="sv"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Stadsmiljö med horisont — 04 och 10</title><style>:root{--paper:#EFE4CB;--ink:#1F1A14;--line:#A08D71}*{box-sizing:border-box}body{margin:0;background:var(--paper);color:var(--ink);font:15px/1.5 system-ui}header,main{max-width:1500px;margin:auto;padding:18px}h1,h2{font-family:Georgia,serif;font-weight:500}h1{font-size:32px}h2{margin-top:40px}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,344px),1fr));gap:16px}figure{padding:10px;margin:0;border:1px solid var(--line);min-width:0}figcaption{font-size:13px;min-height:44px}.viewport{width:320px;max-width:100%;margin:0 auto;overflow:auto}img{display:block;image-rendering:pixelated;image-rendering:crisp-edges;max-width:none}.scroll{overflow:auto;padding-top:12px}.scroll img+img{margin-top:12px}.gray img{filter:grayscale(1)}button{font:inherit;color:var(--ink);background:var(--paper);border:1px solid var(--line);padding:6px 12px;cursor:pointer}a{color:var(--ink)}p{max-width:85ch}.matrix{display:grid;grid-template-columns:repeat(3,minmax(344px,1fr));gap:12px}.matrix-wrap{overflow:auto}.note{border-left:3px solid var(--line);padding-left:12px}@media(max-width:500px){header,main{padding:12px}h1{font-size:27px}.grid{grid-template-columns:1fr}}</style><header><a href="../index.html">← Alla tolv bildspår</a><h1>Stadsmiljö med horisont</h1><p>04 är huvudspåret. Gatan och takgården utvecklas som platser med en horisont bortom husen. Tolv första förslagen finns kvar; här prövas två utvalda kameror med faktisk dataskala.</p><p class="note">Bilderna visas i föreslagen spelskala: <strong>160×96 logiska pixlar ×2 =320×192 CSS-pixlar</strong>. Det ryms i en390px-panel med24px sidmarginal på varje sida och en1px-ram. Logisk1:1 och×4 finns under varje bild. Firefox100%, ingen automatisk bildskalning.</p><button id="gray">Visa gråskala</button> <a href="#status">Tillstånd</a> · <a href="#scale">Storlek och mur</a> · <a href="#variants">Tre varianter</a> · <a href="README.md">Metod och begränsningar</a></header><main>'''
    parts=[intro,'<h2 id="status">Samma stad i fyra tillstånd</h2><p>Normalt: människor och härdar. Svält: tyst gata, stängda luckor, hukande figur och tomma skålar; separat tom-skål-signal förstärker måendet utan att riva hus. Belägring: stängd port och läger i de yttre landöppningarna. Bygge: ställning vid just templet.</p>']
    for camera in RENDER:parts+=['<h3>'+TITLES[camera]+'</h3><div class="grid">',''.join(card(r) for r in rows if r['camera']==camera and 'status' in r['groups']),'</div>']
    parts+=['<h2 id="scale">Datamatris · 1 000 / 4 500 / 10 000 invånare × mur0 / 1 / 3</h2><p>Kolumner: mur0, mur1, mur3. Rader: liten, medelstor, stor stad. På mobil rullar själva matrisen i sidled; varje bild behåller heltalszoom. Population styr kvartersmassor, gatubredd och figurband. Muren styr höjd, portkropp och torn/tinnar.</p>']
    for camera in RENDER:parts+=['<h3>'+TITLES[camera]+'</h3><div class="matrix-wrap"><div class="matrix">',''.join(card(r) for r in sorted(rows,key=lambda r:(r['population'],r['walls'])) if r['camera']==camera and 'scale' in r['groups']),'</div></div>']
    parts+=['<h2 id="variants">Tre varianter per kamera · samma referensdata</h2><p>Huvud: öppen blick mot horisont och helgedom. Djup: starkare förgrund, förskjuten gata respektive närmare takplan. Varma berg: gryningens stegade bergsband från03 bakom samma stadsmiljö.</p>']
    for camera in RENDER:parts+=['<h3>'+TITLES[camera]+'</h3><div class="grid">',''.join(card(r) for r in rows if r['camera']==camera and 'variants' in r['groups']),'</div>']
    parts+=['</main><script>document.querySelector("#gray").onclick=function(){const on=document.body.classList.toggle("gray");this.textContent=on?"Visa färg":"Visa gråskala"}</script></html>']
    (OUT/'index.html').write_text(''.join(parts))
    font=ImageFont.truetype('DejaVuSans.ttf',11)
    for group in ['status','scale','variants']:
        selected=[r for r in rows if group in r['groups']]
        if group=='scale':selected.sort(key=lambda r:(r['camera'],r['population'],r['walls']))
        cols=4 if group=='status' else 3
        sheet=Image.new('RGB',(cols*336,((len(selected)+cols-1)//cols)*228),base.P['light']);d=ImageDraw.Draw(sheet);d.fontmode='1'
        for i,r in enumerate(selected):
            x=i%cols*336;y=i//cols*228
            label=f"{r['camera']} · {r['population']} · mur{r['walls']} · {LABELS[r['state']]} · {r['variant']}"
            d.text((x+8,y+6),label,font=font,fill=base.P['ink'])
            sheet.paste(Image.open(OUT/(r['stem']+'-2x.png')),(x+8,y+28))
        sheet.save(OUT/(group+'-kontakt.png'));ImageOps.grayscale(sheet).save(OUT/(group+'-graskala.png'))
    hashes={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(OUT.glob('*.png')) if not p.name.startswith('firefox-')}
    (OUT/'png-sha256.json').write_text(json.dumps(hashes,indent=2)+'\n')
    print(f'{len(rows)} distinct scenes; {len(hashes)} PNGs')

if __name__=='__main__':main()
