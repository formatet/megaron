#!/usr/bin/env python3
"""Three stronger population grammars for the street camera, same 160x96 raster."""
from pathlib import Path
from PIL import Image,ImageDraw,ImageFont,ImageOps
import importlib.util,hashlib,json
OUT=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('city21b',OUT.parent/'21b/generate.py')
prior=importlib.util.module_from_spec(spec);spec.loader.exec_module(prior)
POPS=[1000,4500,10000]
METHODS={
 'gatuklyfta':('Glesa gårdar → stadsklyfta','Öppna odlingsluckor ersätts av sammanhängande gatufasader och fler våningar. Det stora greppet ligger i gatans väggar och höjd, inte i figurantalet.'),
 'terrasser':('Staden klättrar i terrasser','Storleken bärs av bebyggelsens bredd, antal terrassplan och en gata som går uppför. Liten stad lämnar odling och bergsluttning synliga; stor stad fyller flera sammanhängande nivåer.'),
 'bortom-templet':('Staden fortsätter bortom templet','Förgrunden håller samma mänskliga skala medan en andra stadsdel växer upp på sluttningen bakom helgedomen. Templet blir en plats inne i staden i stället för hela stadens bortre gräns.'),
}

class Street(prior.City):
 def __init__(self,pop,method):
  super().__init__(pop,1,'varma-berg');self.method=method
  self.mask=Image.new('1',(160,96));self.md=ImageDraw.Draw(self.mask)
  self.mass_count=0;self.level_count=0;self.record_mass=False
 def poly(self,points,c,outline=None):
  super().poly(points,c,outline)
  if hasattr(self,'md'):self.md.polygon(points,fill=1 if self.record_mass else 0)
 def rect(self,box,c):
  super().rect(box,c)
  if hasattr(self,'md'):self.md.rectangle(box,fill=1 if self.record_mass else 0)
 def line(self,points,c,width=1):
  super().line(points,c,width)
  if hasattr(self,'md'):self.md.line(points,fill=1 if self.record_mass else 0,width=width)
 def block(self,x,y,w,h,d=3,reverse=False):
  # A modest shared ground shadow ties houses together; roof direction may vary.
  self.poly([(x+2,y+h),(x+w+d+4,y+h-d),(x+w+d+6,y+h-d+3),(x+5,y+h+4)],'rock')
  self.record_mass=True
  if not reverse:
   self.box(x,y,w,h,d)
   self.md.polygon([(x,y),(x+d,y-d),(x+w+d,y-d),(x+w+d,y+h-d),(x+w,y+h),(x,y+h)],fill=1)
  else:
   self.poly([(x,y),(x+w,y),(x+w-d,y-d),(x-d,y-d)],'roof')
   self.rect((x,y+1,x+w,y+h),'shade')
   self.poly([(x-d,y-d),(x,y),(x,y+h),(x-d,y+h-d)],'plaster')
   self.line([(x-d,y-d),(x+w-d,y-d)],'light')
   self.rect((x+w//2,y+h-3,x+w//2+1,y+h),'ink')
   self.md.polygon([(x-d,y-d),(x+w-d,y-d),(x+w,y),(x+w,y+h),(x,y+h),(x-d,y+h-d)],fill=1)
  if w>=16:
   self.rect((x+3,y+3,x+4,y+5),'edge')
  self.record_mass=False
  self.mass_count+=1
 def town_background(self):
  self.mountains()
  self.poly([(0,58),(28,43),(55,36),(93,35),(125,44),(160,57),(160,96),(0,96)],'earth')
 def front(self,lefttop,righttop,leftinner,rightinner):
  self.record_mass=True
  # Continuous frontage is a population signal. The horizon remains in its central aperture.
  self.poly([(0,lefttop+4),(leftinner,48),(leftinner,67),(leftinner-8,96),(0,96)],'plaster')
  self.poly([(0,lefttop),(leftinner+2,44),(leftinner+2,48),(0,lefttop+4)],'roof')
  self.poly([(rightinner,46),(160,righttop+4),(160,96),(rightinner+9,96),(rightinner,66)],'edge')
  self.poly([(rightinner,42),(160,righttop),(160,righttop+4),(rightinner,47)],'roof')
  self.md.polygon([(0,lefttop),(leftinner,44),(leftinner,67),(leftinner-8,96),(0,96)],fill=1)
  self.md.polygon([(rightinner,42),(160,righttop),(160,96),(rightinner+9,96),(rightinner,66)],fill=1)
  # Deep doorways keep the viewer at human scale.
  self.poly([(137,58),(148,54),(148,86),(137,80)],'ink')
  for xx,yy in [(17,max(lefttop+10,54)),(35,max(lefttop+16,53))]:self.rect((xx,yy,xx+4,yy+8),'wood')
  self.record_mass=False
  self.mass_count+=2
 def gate(self,foot=64):self.fortification(40,125,foot,80)
 def finishing(self):
  # Same functional inventory in all populations. Cropped sightlines are camera limits.
  self.flag(108,31)
  self.water([(0,81),(21,74),(37,81),(0,96)])
  self.harbour(0,82)
  self.folk=[(67,83,2),(105,87,2),(83,69,1),(51,90,1),(119,76,1),(76,77,1)][: [2,4,6][self.tier]]
  self.vents=[(x,y) for x,y in [(22,44),(143,42),(42,35),(121,32),(60,26),(100,25)][: [1,3,6][self.tier]]]
  self.features.update(housing_masses=self.mass_count,terrace_levels=self.level_count,people=len(self.folk),hearths=len(self.vents),meal_point=(58,89),hunger_point=(74,87),empty_bowls=[(60,89),(88,90)],shutters=[])
 def overlay_life(self):
  housing=self.mask.copy()
  structure=self.im.copy();layer=Image.new('RGBA',(160,96));self.im=layer;self.d=ImageDraw.Draw(layer)
  for x,y,size in self.folk:self.person(x,y,size)
  for x,y in self.vents:self.line([(x,y),(x-1,y-3),(x+1,y-6)],'smoke')
  self.im=Image.alpha_composite(structure.convert('RGBA'),layer).convert('RGB');self.d=ImageDraw.Draw(self.im)
  self.mask=housing;self.md=ImageDraw.Draw(self.mask)
  return structure,layer

def street_canyon(pop):
 s=Street(pop,'gatuklyfta');s.town_background()
 s.poly([(24,96),(71,50),(91,50),(145,96)],'earth')
 if s.tier==0:
  s.farm(2,59,38);s.farm(115,68,35)
  s.block(14,70,20,13,4);s.block(128,67,18,17,4,True)
  s.block(51,47,12,9,3);s.block(108,50,13,10,3,True)
 else:
  # Backward depth layers, all genuinely visible above the street-side frontage.
  levels=2 if s.tier==1 else 4
  for row in reversed(range(levels)):
   y=38-row*6
   plots=[(39-row*4,y,12,8,3),(109+row*3,y+2,12,9,3)]
   if s.tier==2:plots += [(56,y-2,10,7,3),(95,y-1,11,8,3)]
   for i,values in enumerate(plots):s.block(*values,reverse=i%2==1)
  s.front(37 if s.tier==1 else 17,31 if s.tier==1 else 13,49 if s.tier==1 else 59,115 if s.tier==1 else 106)
  if s.tier==2:
   s.line([(0,40),(53,54)],'shade');s.line([(112,53),(160,35)],'wood')
   for x,y in [(15,31),(30,36),(143,31)]:s.rect((x,y,x+3,y+6),'wood')
 s.level_count=[1,2,4][s.tier]
 s.temple(70,43,22,12);s.gate(64);s.mine(145,41)
 s.finishing();return s

def terraces(pop):
 s=Street(pop,'terrasser');s.town_background()
 s.poly([(46,96),(76,44),(89,43),(118,96)],'light')
 # The ascent itself has depth; the settlements share identical camera and temple anchor.
 for y in range(59,94,5):
  half=round((y-42)*.35)+5;s.line([(82-half,y),(82+half,y)],'shade')
 s.farm(1,68,31);s.mine(147,45)
 levels=[1,3,5][s.tier]
 for row in reversed(range(levels)):
  y=70-row*9;outer=17-row*2
  s.poly([(outer,y+13),(68,y+5),(68,y+8),(outer,y+17)],'rock')
  s.poly([(97,y+5),(149-outer//3,y+12),(149-outer//3,y+16),(97,y+8)],'rock')
  if s.tier==0:
   s.block(22,66,19,14,4);s.block(122,63,19,17,4,True)
   s.farm(37,55,26)
  else:
   for x,w,dy,rev in [(10+row*3,16,3,False),(36+row*2,17,-1,False),(108-row,16,0,True),(132-row*2,15,4,True)]:
    s.block(x,y+dy,w,10 if row<3 else 8,3,rev)
 # Low close roof edges anchor the viewer inside, not above, the city.
 if s.tier>0:
  s.block(0,79,30,23,5);s.block(132,76,26,24,5,True)
 s.level_count=levels;s.temple(72,43,22,12);s.gate(65)
 s.finishing();return s

def beyond(pop):
 s=Street(pop,'bortom-templet');s.town_background()
 s.poly([(40,96),(68,56),(96,56),(132,96)],'earth')
 if s.tier==0:
  s.farm(5,48,50);s.farm(109,53,40)
  s.block(10,69,22,17,4);s.block(130,67,21,18,4,True)
  s.block(47,53,13,10,3)
 else:
  # The background is an actual residential second district, not the fixed horizon silhouette.
  bands=2 if s.tier==1 else 4
  for row in reversed(range(bands)):
   y=43-row*7
   start=47-row*5;end=113+row*5
   xs=list(range(start,end,15 if row%2 else 17))
   for i,x in enumerate(xs):
    s.block(x,y+(i%3)*2,12+(i%2)*2,7+(i%2),3,reverse=i%2==1)
  s.block(17,63,27,22,5);s.block(121,61,29,24,5,True)
  if s.tier==2:
   s.block(0,71,23,27,5);s.block(144,68,20,29,4,True)
 s.level_count=[1,2,4][s.tier]
 s.temple(71,49,22,12);s.gate(70);s.mine(145,42)
 s.finishing();return s

RENDER={'gatuklyfta':street_canyon,'terrasser':terraces,'bortom-templet':beyond}

def main():
 rows=[];font=ImageFont.truetype('DejaVuSans.ttf',11)
 contact=Image.new('RGB',(3*336,3*228),prior.base.P['light']);draw=ImageDraw.Draw(contact);draw.fontmode='1'
 bare=contact.copy();bd=ImageDraw.Draw(bare);bd.fontmode='1'
 for i,(method,fn) in enumerate(RENDER.items()):
  for j,pop in enumerate(POPS):
   s=fn(pop);structure,life=s.overlay_life();stem=f'{method}-p{pop}'
   for scale in [1,2,4]:
    im=s.im if scale==1 else s.im.resize((160*scale,96*scale),Image.Resampling.NEAREST)
    im.save(OUT/(stem+('' if scale==1 else f'-{scale}x')+'.png'))
   structure.save(OUT/(stem+'-structure.png'));life.save(OUT/(stem+'-life.png'));s.mask.save(OUT/(stem+'-housing-mask.png'))
   # Count only residential pixels, not figures, smoke, temple or terrain: measures geometric mass.
   count=sum(1 for v in s.mask.get_flattened_data() if v)
   rows.append(dict(stem=stem,method=method,population=pop,walls=1,features=s.features,housing_pixels=count,horizon_sample=[80,4]))
   label=f'{METHODS[method][0]} · {pop:,} inv'.replace(',',' ')
   x=j*336;y=i*228
   draw.text((x+8,y+6),label,font=font,fill=prior.base.P['ink']);contact.paste(Image.open(OUT/(stem+'-2x.png')),(x+8,y+28))
   bd.text((x+8,y+6),label+' · struktur',font=font,fill=prior.base.P['ink']);bare.paste(structure.resize((320,192),Image.Resampling.NEAREST),(x+8,y+28))
 contact.save(OUT/'kontaktark.png');ImageOps.grayscale(contact).save(OUT/'graskala.png');bare.save(OUT/'struktur-kontaktark.png')
 (OUT/'manifest.json').write_text(json.dumps(dict(size=[160,96],population_bands=POPS,reference=prior.base.DATA,scenes=rows),ensure_ascii=False,indent=2)+'\n')
 page='''<!doctype html><html lang="sv"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Stadens storlek från gatan — 21c</title><style>:root{--paper:#EFE4CB;--ink:#1F1A14;--line:#A08D71}*{box-sizing:border-box}body{margin:0;color:var(--ink);background:var(--paper);font:15px/1.5 system-ui}header,main{max-width:1500px;padding:18px;margin:auto}h1,h2{font-family:Georgia,serif;font-weight:500}h1{font-size:32px}figure{margin:0;padding:10px;min-width:0;border:1px solid var(--line)}figcaption{font-size:13px;min-height:42px}.matrix{display:grid;grid-template-columns:repeat(3,minmax(344px,1fr));gap:14px}.wrap,.scroll{overflow:auto}img{display:block;image-rendering:pixelated;image-rendering:crisp-edges;max-width:none}.viewport{width:320px;margin:auto}.gray img{filter:grayscale(1)}button{font:inherit;padding:6px 12px;background:var(--paper);color:var(--ink);border:1px solid var(--line);cursor:pointer}a{color:var(--ink)}p{max-width:85ch}.scroll img{margin-top:12px}@media(max-width:500px){header,main{padding:12px}h1{font-size:27px}}</style><header><a href="../index.html">← Alla grundspår</a> · <a href="../21b/index.html">Tillstånd och murar</a><h1>Stadens storlek från gatan</h1><p>Tre starkare grepp för 04. Samma tempel, gård2, hamn, varv, gruva och mur1 i alla bilder. Befolkning går från1000 via4500 till10000. Kameran är fast inom respektive grepp. Den lilla staden lämnar luckor; den stora fortsätter i höjd och djup.</p><p>Föreslagen spelskala320×192 (160×96 logiska pixlar×2). Matrisen rullar på mobil; varje bild behåller sin verkliga storlek. Strukturprovet visar samma bilder utan människor eller rök.</p><button id="gray">Visa gråskala</button> <a href="struktur-kontaktark.png">Struktur utan liv/rök</a> · <a href="README.md">Metod och begränsningar</a></header><main>'''
 for method,(title,desc) in METHODS.items():
  page+=f'<h2>{title}</h2><p>{desc}</p><div class="wrap"><div class="matrix">'
  for row in rows:
   if row['method']!=method:continue
   stem=row['stem'];page+=f'<figure><figcaption>{row["population"]:,} invånare · mur1 · {row["features"]["terrace_levels"]} bebyggelseplan</figcaption><div class="viewport"><img src="{stem}-2x.png" width="320" height="192" alt="{title}, {row["population"]} invånare"></div><details><summary>Logisk1:1 och×4</summary><div class="scroll"><img src="{stem}.png" width="160" height="96" alt="Logisk raster"><img src="{stem}-4x.png" width="640" height="384" alt="Fyra gånger större"></div></details></figure>'
  page+='</div></div>'
 page+='</main><script>document.querySelector("#gray").onclick=function(){let on=document.body.classList.toggle("gray");this.textContent=on?"Visa färg":"Visa gråskala"}</script></html>'
 (OUT/'index.html').write_text(page)
 hashes={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(OUT.glob('*.png')) if not p.name.startswith('firefox-')}
 (OUT/'png-sha256.json').write_text(json.dumps(hashes,indent=2)+'\n');print(json.dumps([(r['method'],r['population'],r['housing_pixels']) for r in rows]))

if __name__=='__main__':main()
