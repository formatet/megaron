#!/usr/bin/env python3
"""Build the BILD review page from native captures; never changes screenshots."""
from pathlib import Path
import json,html,hashlib
ROOT=Path(__file__).resolve().parents[1]
root=ROOT/'docs/reviews/byggplan-20261024'
p=json.loads((root/'gallery/proof.json').read_text())
labels={'A1':'Myrines mottagna brev till Leandros — ✉ Reply (1280).','A2':'Efter riktigt Reply-klick: Myrines tråd och svar med samma bud (1280).','A3':'Offer: 28 av Latos 29 korn; varningen visar att en game days måltid inte återstår (1280).','A4':'Myrines mottagna brev — ✉ Reply (390).','A5':'Efter Reply: Myrines tråd och svarsruta (390).','A6':'Offer: samma 28/29-varning på 390.','C1':'Synlig rest host: Dorians skydd, serverns 409-varning och March anyway (1280).','C2':'Samma rest-host-varning och March anyway (390).','D1':'Storm över en bemannad fiskehex: ingen fisk där medan stormen står (1280).','D1m':'Samma fiskestopp, City → Food (390).','G1':'Faktisk byggkvittens, 1800 s/tick: 8 game days · about 4 hours · ready ≈ today … (1280).','G2':'Faktisk byggkvittens på 390: 4 game days · about 2 hours · ready ≈ today … .','D2i':'Efter hela den verkliga animationen: minnesgrått (1280).','D2j':'Efter hela den verkliga animationen: minnesgrått (390).'}
for o in p['observations']:
 for t in o['timeline']:
  a,b=t['before_screenshot_ms']/1000,t['after_screenshot_ms']/1000
  labels[t['code']]=f"Potnias verkliga WS-svar: begärd bild vid {t['target_ms']/1000:.1f}s; screenshot fångades inom {a:.3f}–{b:.3f}s ({o['width']})."
order=['A1','A2','A3','A4','A5','A6','C1','C2','D1','D1m','D2a','D2b','D2c','D2d','D2i','D2e','D2f','D2g','D2h','D2j','G1','G2']
by={s['code']:s for s in p['shots']};assert set(order)==set(by)
page='''<!doctype html><html lang="sv"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Byggplan 2026-10-24 — BILD A/C/D + G</title>
<style>body{font:16px system-ui,sans-serif;margin:24px;line-height:1.5}nav{position:sticky;top:0;background:white;padding:10px;z-index:1;border-bottom:1px solid silver}nav a{margin-right:14px;white-space:nowrap}figure{margin:32px 0;scroll-margin-top:100px}figcaption{margin-bottom:8px}img{max-width:100%;height:auto;border:1px solid silver}button{margin:4px;padding:5px 12px}p{max-width:1000px}small{color:#555}</style>
<h1>Byggplan 2026-10-24 — BILD</h1>
<p>22 oförändrade native Firefox-skärmbilder från <strong>3c984c18 / migration 210 / 1800 s per tick</strong>, på 1280×900 och 390×844. Klicka på bilden för originalfilen vid 1:1. Detta är granskningsunderlag; Timothys BILD-bedömning återstår.</p>
<p><a href="README.md">Metod och avgränsning</a> · <a href="gallery/proof.json">Provenance, HTTP/WS, ljudobservatör och bildtider</a> · <a href="gallery/fixture.sql">Egen lokal scenfixtur</a></p>
<p>D2 är ett riktigt bönesvar via WS: dimma → grönt från fyndet → stegvis blekning → minne. Här avslöjas 19 hex, så vid 4 sekunder har sena hex ännu grönt; extra D2i/j visar hela slutet efter minst 7,8 sekunder. WebAudio-observatören såg playOmen-signaturens 146,8/587,3/622,3/587,3 Hz; ljudkvaliteten bedöms inte av en bild.</p>
<nav>'''
page+=' '.join(f'<a href="#{c}">{c}</a>' for c in order)+'</nav><div><button data-filter="all">Alla</button><button data-filter="1280">1280</button><button data-filter="390">390</button></div>'
for c in order:
 s=by[c];w=s['width'];h=844 if w==390 else 900;url='gallery/'+s['file']
 page+=f'<figure id="{c}" data-width="{w}"><figcaption><strong>{c}</strong> — {html.escape(labels[c])}</figcaption><a href="{url}"><img src="{url}" width="{w}" height="{h}" loading="lazy" alt="{html.escape(labels[c])}"></a></figure>\n'
page+='''<script>document.querySelectorAll('[data-filter]').forEach(b=>b.onclick=()=>document.querySelectorAll('figure').forEach(f=>f.hidden=b.dataset.filter!=='all'&&f.dataset.width!==b.dataset.filter));</script></html>'''
(root/'index.html').write_text(page)
checks=[]
for run in ('final','final-repeat','gallery'):
 d=json.loads((root/run/'proof.json').read_text());assert all(hashlib.sha256((ROOT/f).read_bytes()).hexdigest()==h for f,h in d['source_sha256'].items())
 checks.append({'run':run,'health':d['health'],'tick_seconds':d['tick_seconds'],'production_files':len(d['source_sha256']),'unchanged':True,'page_errors':[o['errors'] for o in d['observations']]})
(root/'final-source-verification.json').write_text(json.dumps(checks,indent=2)+'\n')
print('Gallery:22 native images; three completed captures use same910 production files')
