#!/usr/bin/env python3
"""Offline composition studies. Pillow raster primitives, no resampling except NEAREST.
Run: python3 docs/reviews/stadsvy-alternativ/generate.py
Requires Pillow. All output stays beside this file. No game assets are imported.
"""
from pathlib import Path
from PIL import Image, ImageDraw, ImageOps
import hashlib
import html
import json

OUT = Path(__file__).resolve().parent
DATA = dict(name='Phaistos', population=4500, walls=1,
            buildings=dict(temple=1, farm=2, harbour=1, shipyard=1, mine=1))
P = dict(ink='#1F1A14', light='#EFE4CB', plaster='#D2C3A0', shade='#A08D71',
         roof='#C0A473', edge='#836A44', earth='#C4A578', stone='#B5A992',
         rock='#6E6552', red='#B0442A', blood='#922B21', wood='#6B5334',
         sea='#5E7C8C', deep='#31586A', foam='#A6B9B8', olive='#72784E',
         leaf='#4C5940', field='#B5A062', sky='#C6CBBC', far='#99A391',
         smoke='#938F84', gold='#F9E79F', fire='#E07020')
STATES = ['normal', 'starvation', 'siege', 'construction']
LABELS = dict(normal='Normalt', starvation='Svält', siege='Belägrad', construction='Tempelbygge')

# Composition-specific cameras: every coordinate below is in logical pixels.
class Scene:
    def __init__(self, size=(192,112), palette=None):
        self.p = P | (palette or {})
        self.im = Image.new('RGB',size,self.p['sky'])
        self.d = ImageDraw.Draw(self.im)
        self.anchors = {}; self.people=[]; self.hearths=[]
    def poly(self, points, c, outline=None):
        self.d.polygon(points, fill=self.p.get(c,c), outline=self.p.get(outline,outline))
    def rect(self, box, c): self.d.rectangle(box,fill=self.p.get(c,c))
    def line(self, points, c, width=1): self.d.line(points, fill=self.p.get(c,c),width=width)
    def box(self,x,y,w,h,depth=4, temple=False):
        # Roof plane and two unequal walls; shared neighbourhood shadows, no house outline.
        self.poly([(x,y),(x+w,y),(x+w+depth,y-depth),(x+depth,y-depth)],'light' if temple else 'roof')
        self.rect((x,y+1,x+w,y+h),'plaster' if temple else 'shade')
        self.poly([(x+w,y),(x+w+depth,y-depth),(x+w+depth,y+h-depth),(x+w,y+h)],'shade' if temple else 'edge')
        self.line([(x,y),(x+w,y)],'light' if temple else 'earth')
        self.line([(x,y+h+1),(x+w,y+h+1),(x+w+depth,y+h-depth+1)],'rock')
        self.rect((x+w//2,y+h-3,x+w//2+1,y+h),'ink')
        if w>12:self.rect((x+3,y+3,x+4,y+4),'edge')
    def temple(self,x,y,w=23,h=12):
        if not DATA['buildings'].get('temple'):return
        self.box(x,y,w,h,5,True)
        self.rect((x-2,y+h+1,x+w+3,y+h+2),'stone')
        # Minoan flat roof, broad lintel, tapering red columns; no classical pediment.
        self.rect((x+2,y+3,x+w-2,y+4),'red')
        for xx in [x+4,x+w-5]:
            self.rect((xx,y+4,xx+2,y+h),'red');self.rect((xx+1,y+6,xx+2,y+h),'wood')
        self.rect((x+8,y+5,x+w-8,y+h),'ink')
        self.rect((x+2,y-3,x+4,y-1),'light');self.rect((x+6,y-3,x+8,y-1),'light')
        self.anchors['temple']=(x,y,w,h)
    def flag(self,x,y):
        self.line([(x,y+15),(x,y)],'ink')
        self.poly([(x+1,y),(x+10,y+2),(x+7,y+5),(x+1,y+5)],'red','ink')
        self.rect((x+3,y+2,x+4,y+3),'light')
    def person(self,x,y,scale=1):
        self.rect((x,y-4*scale,x+scale-1,y-3*scale),'wood')
        self.rect((x-scale,y-2*scale,x+scale,y),'red')
        self.line([(x-scale,y+1),(x-scale,y+2*scale)],'ink')
        self.line([(x+scale,y+1),(x+scale,y+2*scale)],'ink')
    def landscape(self,horizon=36,water=True):
        w,h=self.im.size
        self.poly([(0,horizon+7),(0,horizon-8),(22,horizon-13),(46,horizon-4),(77,horizon-18),(120,horizon-9),(154,horizon-16),(w,horizon-5),(w,h)],'far')
        self.poly([(0,horizon+14),(33,horizon+5),(62,horizon+12),(99,horizon+2),(141,horizon+9),(w,horizon),(w,h),(0,h)],'olive')
        if water:self.rect((0,h-20,w,h),'sea')
    def water(self,polygon):
        self.poly(polygon,'sea')
        # Sparse, long horizontal strokes: calm water, not pixel noise.
        w,h=self.im.size
        for x,y,n in [(8,h-8,20),(48,h-4,13),(92,h-12,19),(141,h-6,22),(164,h-16,11)]:
            if self.im.getpixel((min(x,w-1),y))==tuple(bytes.fromhex(self.p['sea'][1:])):
                self.line([(x,y),(min(w-1,x+n),y)],'foam')
    def ship(self,x,y,n=22):
        self.poly([(x,y),(x+n,y),(x+n-4,y+4),(x+4,y+4)],'wood','ink')
        self.line([(x+n//2,y),(x+n//2,y-15)],'ink')
        self.poly([(x+n//2-1,y-14),(x+n//2-9,y-3),(x+n//2-1,y-3)],'light')
        self.line([(x+n//2+1,y-14),(x+n//2+7,y-4)],'edge')
    def harbour(self,x,y):
        if not DATA['buildings'].get('harbour'):return
        self.poly([(x,y),(x+30,y),(x+34,y+4),(x,y+4)],'stone','rock')
        self.rect((x+8,y+3,x+12,y+13),'wood')
        self.line([(x+8,y+4),(x+8,y+14)],'light')
        self.ship(x+16,y+12)
        if DATA['buildings'].get('shipyard'):
            self.line([(x-4,y-3),(x-4,y+4),(x+4,y+7),(x+12,y+7)],'wood',2)
            self.line([(x-6,y-4),(x+10,y+1)],'wood')
            for i in range(3):self.line([(x+1+i*4,y+1),(x+4+i*4,y+5)],'ink')
    def farm(self,x,y,w=32):
        if not DATA['buildings'].get('farm'):return
        self.poly([(x,y),(x+w,y-4),(x+w+6,y+4),(x+5,y+8)],'field')
        for i in range(DATA['buildings']['farm']+2):
            self.line([(x+4,y+i*2),(x+w,y-4+i*2)],'edge')
        self.box(x+2,y-5,8,5,2)
    def mine(self,x,y):
        if not DATA['buildings'].get('mine'):return
        self.poly([(x-8,y+8),(x-5,y-3),(x+5,y-6),(x+13,y+8)],'stone')
        self.rect((x,y,x+5,y+7),'ink')
        self.line([(x-1,y+7),(x-1,y-1),(x+6,y-1),(x+6,y+7)],'wood',2)
        self.line([(x+2,y+8),(x+10,y+13)],'edge')
    def wall(self,points,height=7,gate=None):
        if not DATA['walls']:return
        self.poly(points+[(x,y+height) for x,y in reversed(points)],'rock')
        self.line(points,'stone',3)
        for (a,b) in zip(points,points[1:]):
            x1,y1=a;x2,y2=b
            for i in range(1,max(2,abs(x2-x1)//7)):
                t=i/max(2,abs(x2-x1)//7);xx=round(x1+(x2-x1)*t);yy=round(y1+(y2-y1)*t)
                self.line([(xx,yy+2),(xx,yy+height-1)],'shade')
        if gate:
            x,y=gate;self.rect((x-4,y,x+4,y+height),'ink')
            self.rect((x-6,y-2,x-4,y+height),'stone');self.rect((x+4,y-2,x+6,y+height),'stone')
            self.anchors['gate']=(x,y,height)
    def status(self,state):
        w,h=self.im.size
        # Each status is assembled on transparent layers over the identical structural raster.
        base=self.im; layer=Image.new('RGBA',base.size);self.im=layer;self.d=ImageDraw.Draw(layer)
        if state!='starvation':
            for x,y,s in self.people:self.person(x,y,s)
            for x,y in self.hearths:
                self.line([(x,y),(x-1,y-3),(x+1,y-6),(x,y-8)],'smoke')
        else:
            # No fictitious ruins: empty streets and a closed food stall under the same urban fabric.
            x,y,_=self.people[0] if self.people else (w//2,h-25,1)
            self.line([(x-3,y),(x+5,y)],'wood',2)
            self.line([(x-3,y-5),(x-3,y+2),(x+5,y+2),(x+5,y-5)],'edge')
            self.rect((x-2,y-4,x+4,y-1),'rock')
            self.person(x+13,y+1)
            self.line([(x+12,y-2),(x+15,y)],'wood')
        if state=='siege':
            x,y,hh=self.anchors.get('gate',(w//2,h-26,7))
            self.rect((x-3,y,x+3,y+hh),'wood')
            self.line([(x-3,y+2),(x+3,y+hh-1)],'ink')
            for xx,yy in self.anchors.get('camps',[(18,h-20),(w-33,h-23)]):
                self.poly([(xx-8,yy+6),(xx,yy-3),(xx+9,yy+6)],'blood','ink')
                self.line([(xx,yy-3),(xx,yy+6)],'light')
                self.rect((xx+11,yy+4,xx+14,yy+5),'fire')
                self.line([(xx+12,yy+3),(xx+10,yy-2),(xx+12,yy-6)],'smoke')
        if state=='construction':
            x,y,ww,hh=self.anchors['temple']
            for xx in [x-3,x+ww//2,x+ww+5]:self.line([(xx,y-7),(xx,y+hh+3)],'wood')
            for yy in [y-5,y+2,y+hh-1]:self.line([(x-4,yy),(x+ww+6,yy)],'wood')
            self.line([(x-3,y+hh),(x+ww+5,y-5)],'wood')
            self.line([(x+ww+6,y-4),(x+ww+13,y+hh+2)],'wood')
            for i in range(3):self.line([(x+ww+7+i,y+i*3),(x+ww+10+i,y+i*3)],'stone')
            self.rect((x+ww+8,y+hh+3,x+ww+15,y+hh+5),'stone')
        self.im=Image.alpha_composite(base.convert('RGBA'),layer).convert('RGB');self.d=ImageDraw.Draw(self.im)
        return layer

def slope():
    s=Scene();s.landscape(32)
    s.poly([(0,85),(21,71),(44,54),(84,33),(110,30),(147,48),(192,86),(192,112),(0,112)],'rock')
    s.poly([(0,83),(22,69),(47,51),(86,31),(108,29),(145,46),(186,84),(143,89),(51,89)],'earth')
    s.farm(3,62,27);s.mine(169,48)
    for x,y,w,h,d in [(63,48,15,10,5),(95,48,17,11,4),(47,62,18,11,5),(74,63,17,12,5),(111,61,18,10,5),(29,75,17,9,4),(58,77,20,9,4),(99,77,21,10,5),(129,76,17,9,4)]:s.box(x,y,w,h,d)
    s.temple(87,30,24,12);s.flag(122,29)
    s.poly([(83,54),(91,52),(97,88),(84,88)],'light')
    for y in range(57,83,4):s.line([(84,y),(93,y)],'shade')
    s.wall([(19,80),(49,86),(84,87),(113,87),(153,80)],7,(89,87))
    s.water([(0,99),(44,94),(103,96),(153,87),(192,83),(192,112),(0,112)])
    s.harbour(123,88)
    s.people=[(78,74,1),(94,82,1),(115,90,1),(48,91,1),(135,86,1)]
    s.hearths=[(54,55),(108,69),(135,66)]
    s.anchors['camps']=[(9,73),(172,71)]
    return s

def cluster():
    s=Scene();s.landscape(20,False)
    s.water([(137,43),(192,23),(192,112),(91,112)])
    s.poly([(19,39),(73,18),(119,27),(156,62),(131,90),(74,99),(21,77)],'rock')
    s.poly([(19,35),(73,14),(119,23),(156,58),(131,86),(74,95),(21,73)],'earth')
    s.farm(3,85,42);s.mine(14,27)
    for x,y,w,h,d in [(57,34,14,8,6),(82,32,15,8,6),(37,48,17,8,7),(107,45,17,9,7),(31,65,18,9,7),(63,74,18,8,7),(94,71,18,9,7),(118,61,16,8,6),(58,52,13,8,5)]:s.box(x,y,w,h,d)
    s.temple(77,48,22,11);s.flag(105,28)
    s.wall([(19,48),(20,72),(68,94),(115,85),(148,64)],7,(76,92));s.harbour(145,76)
    s.people=[(73,67,1),(99,64,1),(104,81,1),(49,78,1),(136,77,1)];s.hearths=[(44,42),(113,39)]
    return s

def dawn():
    s=slope()
    # Explicit palette experiment: fixed stepped mapping, never blur or continuous gradient.
    replacements={'#C6CBBC':'#E3C6A0','#99A391':'#B59B89','#72784E':'#8C8273',
                  '#EFE4CB':'#F5D9A2','#D2C3A0':'#C49A75','#A08D71':'#896858',
                  '#C4A578':'#B88C64','#5E7C8C':'#827F8C','#31586A':'#4D566B',
                  '#6E6552':'#584B50','#836A44':'#604B4A'}
    mapping={tuple(bytes.fromhex(a[1:])):tuple(bytes.fromhex(b[1:])) for a,b in replacements.items()}
    s.im.putdata([mapping.get(px,px) for px in list(s.im.get_flattened_data())]);s.d=ImageDraw.Draw(s.im)
    s.rect((15,9,25,18),'gold');s.rect((13,12,27,16),'gold')
    return s

def street():
    s=Scene();s.landscape(37,False)
    s.poly([(0,112),(76,50),(112,50),(192,112)],'earth')
    s.poly([(0,45),(68,50),(68,68),(0,106)],'plaster')
    s.poly([(0,40),(69,46),(72,51),(0,46)],'roof')
    s.poly([(0,46),(15,47),(15,96),(0,106)],'shade')
    s.poly([(124,45),(192,30),(192,110),(124,69)],'edge')
    s.poly([(124,41),(192,25),(192,32),(124,48)],'roof')
    for x,y in [(27,57),(48,54)]:s.rect((x,y,x+5,y+10),'wood')
    s.poly([(153,55),(169,51),(169,88),(153,79)],'ink')
    s.temple(81,41,27,16);s.flag(114,28)
    s.mine(145,26);s.farm(22,38,27)
    s.wall([(71,60),(122,60)],7,(95,60))
    # A side lane opens onto the harbour; size and industry are necessarily less legible.
    s.water([(0,94),(35,82),(60,89),(0,112)]);s.harbour(7,91)
    s.line([(74,84),(55,111)],'shade');s.line([(111,84),(138,111)],'shade')
    s.people=[(80,91,2),(117,93,2),(97,73,1),(61,100,1),(128,81,1)]
    s.hearths=[(44,43),(160,25)]
    return s

def diorama():
    s=Scene();s.rect((0,0,192,112),'light')
    s.poly([(15,61),(99,20),(177,57),(99,104)],'wood','ink')
    s.poly([(15,53),(99,12),(177,49),(99,96)],'earth','rock')
    s.poly([(15,53),(99,96),(99,104),(15,61)],'edge')
    s.poly([(99,96),(177,49),(177,57),(99,104)],'rock')
    s.water([(129,58),(155,45),(177,49),(99,96),(80,85)])
    s.farm(25,50,27);s.mine(139,33)
    for x,y,w,h,d in [(65,38,15,10,6),(45,57,15,10,5),(72,60,19,12,6),(101,64,13,10,5),(112,43,14,10,5)]:s.box(x,y,w,h,d)
    s.temple(86,31,22,12);s.flag(119,21)
    s.wall([(31,61),(72,82),(115,61)],7,(75,81));s.harbour(131,63)
    s.people=[(80,75,1),(100,57,1),(58,67,1),(124,65,1)];s.hearths=[(54,51),(115,37)]
    s.anchors['camps']=[(92,18),(32,47)]
    return s

def panorama():
    s=Scene((240,96));s.landscape(29)
    s.poly([(0,70),(31,60),(66,45),(95,32),(124,25),(153,37),(199,57),(240,65),(240,79),(0,79)],'earth')
    s.farm(3,59,37);s.mine(211,38)
    for x,y,w,h,d in [(52,58,18,10,4),(72,51,17,9,4),(91,45,16,10,4),(130,46,16,10,4),(154,54,18,9,4),(177,61,17,9,4),(92,62,21,9,4),(121,62,18,10,4)]:s.box(x,y,w,h,d)
    s.temple(114,29,24,12);s.flag(148,28)
    s.wall([(40,68),(91,73),(149,73),(204,69)],6,(118,73))
    s.water([(0,83),(63,77),(160,78),(240,75),(240,96),(0,96)]);s.harbour(155,77)
    s.people=[(58,76,1),(105,78,1),(131,77,1),(153,76,1),(194,77,1)];s.hearths=[(76,46),(163,49)]
    return s

def monumental():
    s=Scene();s.rect((0,0,192,112),'light')
    s.poly([(10,77),(46,46),(75,45),(97,23),(132,43),(171,73),(177,89),(30,100)],'olive')
    s.poly([(35,59),(67,42),(94,40),(125,55),(142,77),(67,88),(35,74)],'earth')
    # Five substantial shared masses rather than a sprinkling of stamps.
    for x,y,w,h,d in [(38,58,27,15,9),(69,49,23,14,9),(105,60,24,16,8),(69,74,28,12,6)]:s.box(x,y,w,h,d)
    s.temple(91,29,30,16);s.flag(133,28)
    s.wall([(28,76),(68,91),(140,80)],8,(87,87));s.mine(153,45);s.farm(8,87,28)
    s.water([(138,81),(192,65),(192,112),(70,112)]);s.harbour(143,87)
    s.people=[(75,82,1),(93,79,1),(129,85,1),(115,53,1)];s.hearths=[(49,51),(115,53)]
    return s

def fresco():
    s=Scene(palette={'sky':'#D8BA8E','sea':'#547D85','shade':'#9B6751','plaster':'#E3C29A','olive':'#88844E'})
    s.rect((0,0,192,112),'earth');s.rect((4,4,187,107),'sky')
    # Intentional flattened Minoan register space, with material volume kept locally.
    s.poly([(5,43),(33,32),(64,37),(93,22),(119,36),(152,26),(187,36),(187,94),(5,94)],'olive')
    for x,y,w,h,d in [(18,55,20,15,3),(47,46,22,16,3),(83,53,18,16,3),(112,47,23,15,3),(145,56,20,14,3)]:s.box(x,y,w,h,d)
    s.temple(82,25,29,16);s.flag(121,21);s.mine(163,31);s.farm(8,80,40)
    s.wall([(12,73),(49,76),(102,76),(174,73)],6,(97,76))
    s.water([(5,94),(48,88),(108,90),(155,83),(187,85),(187,107),(5,107)]);s.harbour(127,87)
    s.people=[(30,80,1),(66,80,1),(107,82,1),(145,79,1),(80,65,1)];s.hearths=[(52,40),(150,50)]
    return s

def ravine():
    s=Scene();s.landscape(29)
    s.poly([(0,77),(25,56),(66,33),(108,30),(143,52),(177,79),(192,78),(192,112),(0,112)],'rock')
    s.poly([(0,70),(29,49),(68,25),(105,24),(142,47),(173,73),(132,88),(105,89),(79,58),(48,73),(22,90)],'earth')
    s.poly([(78,55),(91,57),(114,112),(91,112),(69,68)],'ink')
    s.poly([(72,64),(85,58),(107,90),(96,96)],'rock')
    for x,y,w,h,d in [(45,54,20,11,5),(62,40,17,10,5),(103,47,18,12,5),(120,64,19,11,5),(23,70,16,9,4)]:s.box(x,y,w,h,d)
    s.temple(79,24,25,12);s.flag(113,22);s.mine(156,44);s.farm(4,84,30)
    s.wall([(17,77),(47,82),(70,70)],7)
    s.wall([(106,76),(144,84),(162,75)],7,(135,81))
    s.line([(65,69),(113,79)],'stone',5);s.line([(65,72),(113,82)],'rock',3)
    s.water([(0,103),(43,94),(107,105),(166,91),(192,92),(192,112),(0,112)]);s.harbour(144,94)
    s.people=[(66,73,1),(97,78,1),(119,83,1),(138,90,1),(44,82,1)];s.hearths=[(52,48),(129,58)]
    return s

def roofcourt():
    s=Scene();s.landscape(27)
    s.poly([(0,65),(37,43),(82,30),(127,35),(171,57),(192,77),(192,112),(0,112)],'earth')
    s.farm(5,47,30);s.mine(166,37)
    s.box(54,42,25,13,7);s.box(120,44,24,14,7)
    s.temple(86,32,29,15);s.flag(122,23)
    s.poly([(67,59),(124,55),(145,98),(36,98)],'light')
    for x,y,w,h,d in [(19,68,28,20,7),(144,68,29,20,8),(44,91,27,13,7),(111,94,29,12,7)]:s.box(x,y,w,h,d)
    s.line([(62,62),(111,57)],'shade');s.line([(77,61),(66,89)],'shade')
    s.wall([(12,92),(39,97),(78,99),(109,99),(176,90)],7,(91,99))
    s.water([(0,109),(46,107),(120,108),(177,94),(192,93),(192,112),(0,112)]);s.harbour(151,94)
    s.people=[(85,79,1),(102,69,1),(68,91,1),(123,86,1),(93,95,1)];s.hearths=[(28,60),(151,60)]
    return s

def silhouette():
    s=Scene((224,112),palette={'sky':'#D7AA88','far':'#AA8E87','olive':'#79747B',
        'earth':'#67616C','rock':'#363C50','roof':'#9D8578','shade':'#575363',
        'edge':'#3D4053','plaster':'#B59B83','light':'#F0D1A0','stone':'#9E9491',
        'sea':'#565E79','foam':'#89899C','wood':'#35394A','smoke':'#9F8E90'})
    # Intentional broad stepped sky, thin skyline and a large quiet water foreground.
    s.rect((0,27,224,48),'#C49985');s.rect((0,49,224,61),'#AD8D84')
    s.rect((26,14,40,26),'light');s.rect((24,17,42,23),'light')
    s.poly([(0,63),(30,55),(67,61),(97,50),(132,48),(167,55),(201,43),(224,51),(224,112),(0,112)],'far')
    s.poly([(25,86),(61,67),(99,54),(141,40),(173,47),(202,70),(224,81),(224,112),(25,112)],'rock')
    s.poly([(28,83),(66,63),(100,49),(143,36),(172,43),(205,69),(224,77),(193,88),(83,92)],'earth')
    s.farm(38,76,29);s.mine(198,59)
    for x,y,w,h,d in [(82,64,15,13,3),(103,54,17,12,3),(125,49,14,11,3),(162,58,18,12,3),(144,73,22,10,3),(111,72,17,11,3),(67,77,15,9,3)]:s.box(x,y,w,h,d)
    s.temple(142,36,24,11);s.flag(176,34)
    # Light through occupied rooms is a vitality layer in the production proposal.
    s.wall([(58,86),(102,89),(153,88),(201,82)],7,(127,89))
    s.water([(0,94),(72,94),(132,98),(191,87),(224,88),(224,112),(0,112)])
    s.harbour(177,88)
    s.people=[(90,91,1),(146,91,1),(162,91,1),(176,86,1),(70,90,1)];s.hearths=[(108,48),(168,53)]
    return s

def roofplan():
    s=Scene();s.rect((0,0,192,112),'olive')
    s.poly([(12,38),(58,13),(117,16),(145,42),(137,75),(95,100),(36,90),(9,66)],'rock')
    s.poly([(12,33),(58,8),(117,11),(145,37),(137,70),(95,95),(36,85),(9,61)],'earth')
    # Almost orthographic roof masses around an irregular open court.
    for pts in [[(37,25),(68,18),(70,37),(47,43)],[(79,18),(111,20),(107,34),(80,32)],
                [(21,45),(40,39),(47,54),(24,64)],[(40,61),(57,56),(63,78),(45,79)],
                [(99,48),(123,40),(130,59),(109,66)],[(71,72),(99,65),(108,80),(83,88)]]:
        shadow=[(x+2,y+3) for x,y in pts];s.poly(shadow,'rock');s.poly(pts,'roof');s.line(pts[:2],'light')
    s.poly([(63,44),(87,38),(100,64),(69,71),(57,58)],'light')
    s.temple(69,39,22,10);s.flag(105,32)
    s.mine(163,24);s.farm(4,89,33)
    s.wall([(9,61),(36,85),(95,95),(137,70)],6,(105,88))
    s.water([(156,41),(192,26),(192,112),(97,112),(139,84)])
    s.harbour(147,74)
    s.people=[(66,60,1),(93,61,1),(72,76,1),(119,75,1),(124,85,1)];s.hearths=[(38,25),(96,20)]
    return s

ALTS = [
('01-hamnsluttning','Hamnsluttningen',slope,
 'Låg kamera från vattnet: staden klättrar i terrasser och trappan leder blicken till kultbyggnaden. Djupa gemensamma skuggor och ljusa trappor prövar större valörspann än den gamla lerfamiljen; ljuset följer övre vänster. Terrasser och kvartersmassor väljs i befolkningsband, medan mur, gård, gruva, hamn och varv är egna datastyrda moduler.'),
('02-hojdklunga','Den täta höjdklungan',cluster,
 'Hög sned kamera samlar bostäder kring en liten helgedom och lämnar en tydlig markyta innanför muren. Konturhierarkin följs men en sammanhängande kvartersskugga ersätter kontur runt varje hus. En deterministisk terrängankrad kvartersgraf kan fyllas i täthetsband; byggnadsnivåer byter modulernas silhuett.'),
('03-gryning','Gryning över hamnen',dawn,
 'Samma hamnsluttning används som kontrollerat ljus- och palettprov: himmel och vatten går i varma respektive violetta steg. Det utmanar kravet på exakt samma palett på två avstånd men behåller former och ljusriktning. En gemensam tabell remappar materialtonerna efter scenens ljusläge; speldata och placering är identiska med 01.'),
('04-gatuperspektiv','Gatan mot helgedomen',street,
 'Låg gatukamera ger stora mänskliga former, mörka dörrar och en tydlig målpunkt bortom porten. Det bryter den fullständiga stadsöverblicken: befolkningsstorlek och hamn blir svagare, och måste stödjas av paneltext. Perspektivmallar kan väljas från gatunät och tempelposition, med synliga sidoglimtar av gård, gruva och hamn; detta är det mest begränsade informationsalternativet.'),
('05-diorama','Det skurna stadskortet',diorama,
 'Ett litet stycke kustland lyfts ur marken; mörk snittyta ger tyngd utan extra dekor. Det prövar en kontur runt själva markkortet, i strid med regeln om konturlös mark, men bostäderna behåller halvkontur. En lokal höjdmodell och en packad kvartersgraf kan klippas mot kortets polygon, med hamn i kustkanten och byggnader på egna ankare.'),
('06-panorama','Hamnstadens långa strand',panorama,
 'Bred 240×96-panorama gör kust och varv till förgrund och låter bebyggelsen växa bakom. Den ändrar proportionen och ger mindre höjd per modul men mer sammanhängande landskap. Kustprofilen styr strändernas rytm, befolkningsband fyller sluttningens massor och byggnadsmoduler ansluter till väg eller kaj.'),
('07-monumental','Fyra kvarter och ett tempel',monumental,
 'Några stora gemensamma huskroppar ger en nästan heraldisk stad med tydlig mur och starka skuggor. Regeln om små individuella hus utmanas genom att en form får betyda ett helt kvarter; tempel, gruva och varv behåller egna silhuetter. Befolkningsband styr massornas antal och storlek, medan funktionsdata styr tillägg utan ett hus per datarad.'),
('08-fresk','Den minoiska frisen',fresco,
 'En inramad, nästan frontal bild lånar minoisk registerrytm och röd puts utan klassisk marmor. Den utmanar en gemensam kamera och realistiskt djup men behåller volym lokalt i varje vägg och tak. En registerlayout kan placera kvarter efter stadsdel, och spelstatus kan målas i ett separat främre register; risken är ett mer dekorativt än geografiskt uttryck.'),
('09-ravin','Staden över ravinen',ravine,
 'En mörk spricka delar stadsmassan och en smal passage binder ihop den: terrängen bestämmer kompositionen. Den bryter den obrutna stadssilhuetten och ger marken mycket större visuell vikt. En lokal terrängprofil kan styra två kvartersklungor och vägar; ravin och passage är här kompositionsprov, inte påståenden om Phaistos eller en ny spelmekanik.'),
('10-takgard','Över taken in mot gården',roofcourt,
 'Nära tak i förgrunden ramar in en ljus gemensam gård och en liten helgedom på höjden. Avsiktlig överlappning bryter kravet att varje modul ska synas helt, men skapar djup genom tre tydliga skalplan. En siktespunkt mot tempel och gård kan styra placeringen, med lågprioriterade bostäder som inramning och reserverade öppningar för hamn, gruva och status.'),
('11-skymningssiluett','Staden som horisont',silhouette,
 'En tunn stadsprofil mot bandad skymningshimmel lämnar stor stilla vattenyta framför staden. Svala sammanhängande skuggor utmanar den varma materialpaletten och konturen blir mest negativ silhuett; solens ljus kommer fortfarande uppifrån vänster. En höjdprofil och befolkningsband formar horisonten, medan murkrön, helgedom, gruva och riggning behåller sina datastyrda avbrott.'),
('12-takplan','Taklandskapet och den vita gården',roofplan,
 'Nästan lodrät kamera gör staden till sammanhängande takfält kring en ljus oregelbunden gård. Det prövar kraftigt reducerade hus utan väggfasader och låter riktningarna följa kvarterskanterna i stället för en gemensam kubstämpel. En cellindelad kvartersplan kan fyllas i befolkningsband och lämna fri sikt kring funktionsmoduler; risken är att bilden åter blir en planritning.'),
]
FAVOURITES = {'01-hamnsluttning','05-diorama'}

def save_pair(im,stem):
    im.save(OUT/(stem+'.png'))
    im.resize((im.width*4,im.height*4),Image.Resampling.NEAREST).save(OUT/(stem+'-4x.png'))

def main():
    manifest={'fixture':DATA,'favourites':sorted(FAVOURITES),'images':[]}
    thumbs=[];grays=[];cards=[];status_cards=[]
    for ident,title,fn,desc in ALTS:
        base=fn();save_pair(base.im,ident+'-structure') if ident in FAVOURITES else None
        states=STATES if ident in FAVOURITES else ['normal']
        for state in states:
            s=fn();layer=s.status(state);stem=ident+'-'+state;save_pair(s.im,stem)
            if ident in FAVOURITES:layer.save(OUT/(stem+'-layer.png'))
            manifest['images'].append(dict(id=ident,state=state,file=stem+'.png',size=list(s.im.size),anchors=s.anchors))
            if state=='normal':
                thumbs.append((title,s.im.copy()));grays.append((title,ImageOps.grayscale(s.im).convert('RGB')))
            card=f'<figure><figcaption>{html.escape(title)} — {LABELS[state]}</figcaption><div class="logical"><img src="{stem}.png" width="{s.im.width}" height="{s.im.height}" alt="{html.escape(title)}: {LABELS[state]}"></div><details><summary>×4 — närmaste granne</summary><div class="large"><img src="{stem}-4x.png" width="{s.im.width*4}" height="{s.im.height*4}" alt="Samma bild fyra gånger större"></div></details>'
            if state=='normal':cards.append(card+f'<p>{html.escape(desc)}</p><a href="{stem}.png">Logisk PNG</a> · <a href="{stem}-4x.png">×4 PNG</a></figure>')
            if ident in FAVOURITES:status_cards.append(card+'</figure>')
    for name,items in [('kontaktark',thumbs),('graskala',grays)]:
        sheet=Image.new('RGB',(2*272,((len(items)+1)//2)*150),'#EFE4CB');d=ImageDraw.Draw(sheet);d.fontmode='1'
        for i,(title,im) in enumerate(items):
            x=(i%2)*272;y=(i//2)*150;d.text((x+8,y+5),title,fill='#1F1A14');sheet.paste(im,(x+8,y+26))
        save_pair(sheet,name)
    (OUT/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    page='''<!doctype html><html lang="sv"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Phaistos — tolv stadsvyer</title>
<style>:root{--paper:#EFE4CB;--ink:#1F1A14;--muted:#6E6552;--border:#A08D71}*{box-sizing:border-box}body{margin:0;background:var(--paper);color:var(--ink);font:16px/1.5 system-ui,sans-serif}header,main{max-width:1500px;margin:auto;padding:24px}h1{font:500 36px/1.15 Georgia,serif;margin:12px 0}h2{font:500 27px Georgia,serif;margin-top:42px}p{max-width:80ch}nav a{margin-right:18px;color:var(--ink)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,340px),1fr));gap:18px}figure{margin:0;padding:16px;border:1px solid var(--border);min-width:0}figcaption{font-weight:650;margin-bottom:12px}.logical{min-height:128px;display:flex;align-items:center;justify-content:center;background:var(--paper)}img{image-rendering:pixelated;image-rendering:crisp-edges;max-width:none;display:block}figure p{font-size:14px}.large{overflow:auto;padding-top:12px}summary{cursor:pointer;color:var(--muted)}a{color:var(--ink)}.status{grid-template-columns:repeat(auto-fit,minmax(min(100%,300px),1fr))}.note{border-left:3px solid var(--muted);padding-left:16px}button{font:inherit;padding:6px 12px;background:var(--paper);color:var(--ink);border:1px solid var(--muted);cursor:pointer}.gray img{filter:grayscale(1)}@media(max-width:500px){header,main{padding:16px}h1{font-size:28px}figure{padding:12px}}</style>
<header><small>MEGARON · BILDSTUDIE 21 · INGEN LIVEÄNDRING</small><h1>En stad, tolv olika blickar</h1><p>Phaistos-lik kuststad · 4 500 invånare · mur 1 · tempel 1 · gård 2 · hamn · varv · gruva. Samma byggnadsdata i alla bilder. Terräng och kamera varierar avsiktligt.</p><nav><a href="#alternativ">Alla tolv</a><a href="#tillstand">Två favoriter, fyra tillstånd</a><a href="regler.md">Regelprover</a><a href="README.md">Metod och begränsningar</a></nav><p class="note">1:1 här betyder en logisk pixel per CSS-pixel vid Firefox 100 %. Öppna ×4 för detaljer; bilden får då rulla i sidled. Bilderna är förslag inför Timothys val. Ingen riktning är godkänd.</p><button id="gray" aria-pressed="false">Visa gråskala</button> <button id="enlarge" aria-pressed="false">Öppna alla ×4</button></header><main><h2 id="alternativ">Tolv grundgrepp · logisk 1:1</h2><div class="grid">'''+''.join(cards)+'''</div><h2 id="tillstand">Två preliminära favoriter · samma stad i fyra tillstånd</h2><p>01 ger landskap och djup; 05 ger kompakt överblick. Svält behåller byggnader men tar bort härdar och gatuliv, belägring stänger porten och lägger till läger utanför muren, bygge lägger ställning vid just templet.</p><div class="grid status">'''+''.join(status_cards)+'''</div><h2>Baslinje · oförändrad renderer</h2><p>Den befintliga renderern ritad med samma syntetiska byggnadsdata, i dess vanliga ×2-skala. Detta är ingen bild från en levande stad.</p><div class="large"><img src="baseline-live-renderer.png" width="320" height="152" alt="Befintlig stadsvy med referensfixturen"></div><h2>Jämför utan beskrivningar</h2><p><a href="kontaktark.png">Kontaktark i logisk skala</a> · <a href="graskala.png">Kontaktark i gråskala</a> · <a href="manifest.json">Referensdata och filmanifest</a></p></main><script>document.querySelector('#enlarge').onclick=function(){const on=this.getAttribute('aria-pressed')!=='true';document.querySelectorAll('details').forEach(d=>d.open=on);this.setAttribute('aria-pressed',String(on));this.textContent=on?'Stäng alla ×4':'Öppna alla ×4'};document.querySelector('#gray').onclick=function(){const on=document.body.classList.toggle('gray');this.setAttribute('aria-pressed',String(on));this.textContent=on?'Visa färg':'Visa gråskala'};</script></html>'''
    (OUT/'index.html').write_text(page)
    hashes={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(OUT.glob('*.png')) if not p.name.startswith(('firefox-', 'baseline-'))}
    (OUT/'png-sha256.json').write_text(json.dumps(hashes,indent=2)+'\n')
    print(f'{len(ALTS)} alternatives, {len(manifest["images"])} scenes, {len(hashes)} PNGs')

if __name__=='__main__':main()
