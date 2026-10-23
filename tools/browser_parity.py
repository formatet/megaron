#!/usr/bin/env python3
"""Webbläsarparitet: huvudytorna i Firefox/Chromium/WebKit, desktop + 390 px.

    python3 tools/browser_parity.py [--out DIR] [--browsers firefox chromium webkit]

Loggar in i acceptansvärlden (tools/acceptance.sh) som en riktig spelare, klickar
fram varje yta via sin riktiga knapp och MÄTER i stället för att jämföra bilder.
En andra spelare UTAN stad (PANEL_HOST_USER, skapas med `tools/acceptance.sh player
Nomad`) mäter Host-panelen — det första en ny Wanax möter — öppnad genom ett klick
på hostens serverägda fysiska plats, med Details stängd och öppen. Mätningen:

  - horisontell scroll på sidan
  - ytans ruta utanför viewporten
  - synliga kontroller/text som sticker ut ur ytans ruta (klippta)
  - knappar vars mitt täcks av något annat (går inte att trycka på)
  - JS-fel (pageerror) per yta
  - ytans huvudknapp (t.ex. "Found the metropolis here") utanför bild eller täckt
  - ytans innehållshöjd per motor — avviker en motor > 15 % från medianen flaggas den

Skärmdumpar sparas per motor/läge/yta så att en flaggad rad kan ses sida vid sida.
Exitkod 1 om någon fynd-rad finns. Allt som sägs om ytorna gäller den commit
`tools/acceptance.sh provenance` skriver ut.
"""
import argparse
import json
import os
import pathlib
import statistics
import sys
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright

HOST = os.environ.get("PANEL_HOST", "http://localhost:8097")
USER = os.environ.get("PANEL_USER", "Agamemnon")
PW = os.environ.get("PANEL_PW", "acceptance-pw-123")
HOST_USER = os.environ.get("PANEL_HOST_USER", "Nomad")

# yta → (hur den öppnas, selektorn för dess ruta)
SURFACES = {
    "map": (None, "#hex-canvas"),
    "map-river": ("@river-map", "#hex-canvas"),
    "river": ("@river", "#inspect-panel"),
    "city-food": ("@food", "#drawer-city"),
    "city": ("#trig-city", "#drawer-city"),
    "war": ("#trig-war", "#drawer-war"),
    "diplomacy": ("#trig-diplomacy", "#drawer-diplomacy"),
    "economy": ("#trig-economy", "#drawer-economy"),
    "kult": ("#trig-kult", "#drawer-kult"),
    "report": ("#trig-report", "#drawer-report"),
    "notif": ("#gt-notif-btn", "#drawer-notif"),
    "codex": ("#gt-codex-btn", "#codex-panel"),
    "account": ("#gt-wanax", "#account-window-overlay .dispatch-window"),
}
# Host-fasen: centrera på hostens serverägda fysiska hex och klicka dess markör.
# Kamerans mitt är inte en garanti för vilken hex som träffas. Tredje fältet: knappen som MÅSTE gå att trycka på.
HOST_SURFACES = {
    "host": ("@host", "#inspect-panel", "#ip-settle-btn"),
    "host-details": ("#ip-host-details > summary", "#inspect-panel", "#ip-settle-btn"),
}
RICH_SURFACES = {
 "rich-production": ("#trig-city", "#drawer-city"),
 "rich-food": ("@rich-food", "#drawer-city"),
 "rich-buildings": ('.dtab[data-tab="byggnader"]', "#drawer-city"),
 "rich-garrison": ('.dtab[data-tab="garnison"]', "#drawer-city"),
}
MODES = {"desktop": (1280, 900), "mobile": (390, 844)}

MEASURE = r"""
([sel, reach]) => {
  const box = document.querySelector(sel);
  const out = {found: !!box, page_hscroll: document.documentElement.scrollWidth > innerWidth + 1};
  if (!box) return out;
  const b = box.getBoundingClientRect();
  out.box = {left: Math.round(b.left), right: Math.round(b.right), top: Math.round(b.top), bottom: Math.round(b.bottom)};
  out.box_outside = b.left < -1 || b.right > innerWidth + 1;
  // rutan har fast höjd; det är innehållet i dess scrollande kropp som kan skilja mellan motorer
  out.content_height = (box.querySelector('.drawer-body, .dw-body, #codex-body, .inspect-body') || box).scrollHeight;
  // en canvas ritar i sina egna pixlar: de måste följa behållaren, annars blir kartan en remsa
  if (box.tagName === 'CANVAS') out.canvas_mismatch = box.width !== box.parentElement.clientWidth || box.height !== box.parentElement.clientHeight
    ? `canvas ${box.width}×${box.height}, behållare ${box.parentElement.clientWidth}×${box.parentElement.clientHeight}` : null;
  const visible = e => { const s = getComputedStyle(e); const r = e.getBoundingClientRect();
    return s.visibility !== 'hidden' && s.display !== 'none' && r.width > 0 && r.height > 0; };
  const label = e => (e.id ? '#' + e.id : e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.trim().split(/\s+/).join('.') : '')) + ' «' + (e.textContent || e.value || '').trim().slice(0, 40) + '»';
  // Scrolled content retains a DOM rectangle behind the fixed header/brief.
  // A centre outside an ancestor's clipping viewport is not a visible control.
  const centreExposed = (e, r) => {
    const x=r.left+r.width/2, y=r.top+r.height/2;
    for (let p=e.parentElement; p; p=p.parentElement) {
      const s=getComputedStyle(p), b=p.getBoundingClientRect();
      if (s.overflowX!=='visible' && (x<b.left || x>b.right)) return false;
      if (s.overflowY!=='visible' && (y<b.top || y>b.bottom)) return false;
    }
    return true;
  };
  const sticking = [], clipped = [], covered = [];
  for (const e of box.querySelectorAll('button, input, select, textarea, a, span, td, th, label, div, p, h1, h2, h3, h4, li')) {
    if (!visible(e)) continue;
    const r = e.getBoundingClientRect();
    // utanför rutan i sidled (vertikalt hanteras rutan med scroll)
    if (r.right > b.right + 1 || r.left < b.left - 1) {
      // räkna bara det yttersta elementet som sticker ut
      const p = e.parentElement; const pr = p && p.getBoundingClientRect();
      if (!pr || !(pr.right > b.right + 1 || pr.left < b.left - 1)) sticking.push(label(e) + ` [${Math.round(r.left)}..${Math.round(r.right)}]`);
    }
    // text som klipps utan avsiktlig ellips
    const s = getComputedStyle(e);
    if (e.children.length === 0 && (e.textContent || '').trim() && e.scrollWidth > e.clientWidth + 1 &&
        s.overflowX !== 'visible' && s.textOverflow !== 'ellipsis' && s.overflowX !== 'auto' && s.overflowX !== 'scroll')
      clipped.push(label(e) + ` [${e.scrollWidth}>${e.clientWidth}]`);
    // knappar i viewporten som inte går att träffa
    if (centreExposed(e,r) && e.matches('button, a, select, input') && r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth &&
        r.top >= b.top && r.bottom <= b.bottom) {
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      if (hit && !e.contains(hit) && !hit.contains(e)) covered.push(label(e) + ' ← ' + label(hit));
    }
  }
  // huvudknappen: helt i bild och träffbar i sin mitt — annars når spelaren inte handlingen
  if (reach) {
    const k = document.querySelector(reach);
    if (!k || !visible(k)) out.unreachable = reach + ' saknas';
    else {
      const r = k.getBoundingClientRect();
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      if (r.top < 0 || r.bottom > innerHeight || r.left < 0 || r.right > innerWidth)
        out.unreachable = label(k) + ` utanför bild [${Math.round(r.left)},${Math.round(r.top)}..${Math.round(r.right)},${Math.round(r.bottom)}]`;
      else if (!hit || !(k.contains(hit) || hit.contains(k))) out.unreachable = label(k) + ' täckt av ' + (hit ? label(hit) : 'inget');
    }
  }
  Object.assign(out, {sticking, clipped, covered});
  return out;
}
"""


def login(user):
    req = urllib.request.Request(f"{HOST}/api/v1/auth/login",
                                 json.dumps({"username_or_email": user, "password": PW}).encode(),
                                 {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=15) as r:
        d = json.load(r)
    return d.get("access_token") or d["token"]


def run_engine(pw, engine, token, out, surfaces=SURFACES, close_drawers=True):
    rows = []
    browser = getattr(pw, engine).launch()
    version=browser.version
    for mode, (w, h) in MODES.items():
        ctx = browser.new_context(viewport={"width": w, "height": h}, locale="en-GB",
                                  timezone_id="Europe/Stockholm", has_touch=mode == "mobile")
        ctx.add_cookies([{"name": "poleia_token", "value": token, "url": HOST}])
        ctx.add_init_script(f"localStorage.setItem('poleia_token', {json.dumps(token)})")
        page = ctx.new_page()
        errors = []
        city_payloads=[]
        def record_city(response):
            if '/provinces/' in response.url and response.status==200:
                try:
                    data=response.json()
                    if isinstance(data,dict) and 'settlement' in data:city_payloads.append(data['settlement'])
                except Exception:pass
        page.on('response',record_city)
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
        page.goto(f"{HOST}/play", wait_until="networkidle")
        page.wait_for_function("() => window.centreOn !== undefined", timeout=20000)
        page.evaluate("""async()=>{window.__parityState=(await import('/static/js/megaron/state.js')).State;window.__parityMap=await import('/static/js/megaron/render/map.js');}""")
        page.wait_for_function("__parityState.tileData.length > 0 && __parityMap.markrasterStats.pending===0",timeout=30000)
        page.wait_for_timeout(800)
        for name, (trigger, sel, *reach) in surfaces.items():
            before = 0 if name == next(iter(surfaces)) else len(errors)
            if trigger == "@host":
                # A camera clamp/remembered location can put the Host off centre.
                # Click its authoritative physical hex, not the canvas midpoint.
                host = page.evaluate("""async () => {
                    const {physicalHostState} = await import('/static/js/megaron/ui/host_state.js');
                    return physicalHostState(__parityState.founderPhase, __parityState.unitsData);
                }""")
                if not host:
                    raise RuntimeError("no physical Host state for the Host audit account")
                page.evaluate("t=>window.centreOn(t.q,t.r)", host)
                page.wait_for_function("__parityMap.markrasterStats.pending===0", timeout=30000)
                page.wait_for_timeout(400)
                point = page.evaluate("t=>{const p=__parityMap.hexPx(t.q,t.r),s=__parityState;return {x:s.camera.x+p.x*__parityMap.SCALE*s.camera.zoom,y:s.camera.y+p.y*__parityMap.SCALE*s.camera.zoom}}", host)
                el = page.locator('#hex-canvas')
                el.tap(position=point) if mode == "mobile" else el.click(position=point)
                page.locator('#ip-host-state').wait_for(state='visible', timeout=10000)
                page.wait_for_timeout(900)
            elif trigger in ("@river", "@river-map"):
                river = page.evaluate("() => __parityState.tileData.find(t=>t.river_name && t.tier!=='fog' && t.terrain.startsWith('river'))")
                if not river: raise RuntimeError("no known named river in this player's actual map")
                page.evaluate("t=>window.centreOn(t.q,t.r)",river)
                page.wait_for_function("__parityMap.markrasterStats.pending===0",timeout=30000)
                page.wait_for_timeout(400)
                if trigger=="@river":
                    point=page.evaluate("t=>{const p=__parityMap.hexPx(t.q,t.r),s=__parityState;return {x:s.camera.x+p.x*__parityMap.SCALE*s.camera.zoom,y:s.camera.y+p.y*__parityMap.SCALE*s.camera.zoom}}",river)
                    el=page.locator('#hex-canvas');el.tap(position=point) if mode=="mobile" else el.click(position=point)
                    page.wait_for_timeout(900)
            elif trigger in ("@food","@rich-food"):
                if trigger=="@food":
                    el=page.locator('#trig-city');el.tap() if mode=="mobile" else el.click()
                page.locator('.harvest-blessing').wait_for(state='visible',timeout=10000)
                page.locator('.harvest-blessing').scroll_into_view_if_needed()
                page.wait_for_timeout(400)
            elif trigger:
                # Touch input for mobile, including Firefox's canvas Pointer Events.
                el=page.locator(trigger).first
                el.tap() if mode=="mobile" else el.click()
                page.wait_for_timeout(900)
            m = page.evaluate(MEASURE, [sel, reach[0] if reach else None])
            m.update(engine=engine, version=version, mode=mode, surface=name, errors=errors[before:])
            m['worker']=page.evaluate("()=>({...__parityMap.markrasterStats})")
            if name in ('map','map-river'):
                m['map_facts']=page.evaluate("()=>({known_rivers:__parityState.tileData.filter(t=>t.river_name && t.tier!=='fog').length,cities:__parityState.provinceData.length,tiers:[...new Set(__parityState.tileData.map(t=>t.tier))],zoom:__parityState.camera.zoom})")
            if name=='river':
                m['field_spacing']=page.evaluate("""()=>[...document.querySelectorAll('#ip-body .inspect-row')].filter(row=>{const r=row.getBoundingClientRect();return r.width>0 && row.children.length===2}).map(row=>{const a=row.children[0].getBoundingClientRect(),b=row.children[1].getBoundingClientRect();return {label:row.children[0].textContent,gap:b.left-a.right}})""")

                m['river']={'q':river['q'],'r':river['r'],'name':river['river_name'],'title':page.locator('#ip-name').inner_text(),'terrain':page.locator('#ip-terrain').inner_text()}
                if river['river_name'] not in m['river']['title'] or river['river_name'] in m['river']['terrain']:m['errors'].append('river name/title/terrain mismatch')
            if name in ('host','host-details'):
                row=page.locator('.inspect-row').filter(has_text='Ships:')
                m['ships']=row.inner_text();m['ships_box']=row.bounding_box()
                b=m['ships_box'];m['ships_visible']=bool(b and b['y']>=0 and b['y']+b['height']<=h)
                if not m['ships_visible']:m['errors'].append('Ships row not visible')
            if name in ('city-food','rich-food'):
                line=page.locator('.harvest-blessing');m['harvest']=line.inner_text();m['harvest_box']=line.bounding_box();m['harvest_api']=city_payloads[-1].get('harvest_blessing') if city_payloads else None
                if 'days left' not in m['harvest'] or 'fish' in m['harvest'] or not m['harvest_api']:m['errors'].append('harvest display mismatch')
            if name=='rich-buildings':m['built_rows']=page.locator('#city-bld-sec .goods-mini tr').count()
            if name=='rich-garrison':m['garrison_rows']=page.locator('#city-gar-sec').inner_text()
            page.screenshot(path=str(out / f"{engine}-{mode}-{name}.png"))
            rows.append(m)
            if trigger and close_drawers:  # stäng igen (Host-ytorna bygger på varandra) med samma knapp, sedan Escape som reserv
                page.keyboard.press("Escape")
                page.wait_for_timeout(200)
                if page.evaluate(f"(() => {{ const e = document.querySelector({json.dumps(sel)}); return e && e.getBoundingClientRect().width > 0 && getComputedStyle(e).visibility !== 'hidden' && e.classList.contains('open'); }})()"):
                    if trigger.startswith('@'):
                        if trigger=='@river':page.evaluate('__parityMap.closeInspect()')
                        elif trigger=='@food':page.locator('#trig-city').click()
                    else:page.locator(trigger).first.click(force=True)
                    page.wait_for_timeout(300)
        ctx.close()
    browser.close()
    return rows


def findings(rows):
    out = []
    for r in rows:
        tag = f"{r['engine']:8} {r['mode']:7} {r['surface']:9}"
        if not r["found"]:
            out.append(f"{tag} ytan hittades inte")
            continue
        if r["page_hscroll"]: out.append(f"{tag} sidan scrollar i sidled")
        if r["box_outside"]: out.append(f"{tag} rutan utanför viewporten {r['box']}")
        if r.get("canvas_mismatch"): out.append(f"{tag} {r['canvas_mismatch']}")
        for field in r.get("field_spacing",[]):
            if field["gap"]<1:out.append(f"{tag} label/value touch: {field}")
        if r.get("unreachable"): out.append(f"{tag} går inte att trycka på: {r['unreachable']}")
        for k in ("sticking", "clipped", "covered"):
            for x in r[k]: out.append(f"{tag} {k}: {x}")
        for e in r["errors"]: out.append(f"{tag} JS-fel: {e[:160]}")
    # innehållshöjd per motor, samma yta och läge
    for mode in MODES:
        for s in sorted({r["surface"] for r in rows}):
            hs = {r["engine"]: r.get("content_height") for r in rows if r["mode"] == mode and r["surface"] == s and r.get("content_height")}
            if len(hs) < 2: continue
            med = statistics.median(hs.values())
            for eng, v in hs.items():
                if med and abs(v - med) / med > 0.15:
                    out.append(f"{eng:8} {mode:7} {s:9} höjd {v} mot median {med:.0f} ({hs})")
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", type=pathlib.Path, default=pathlib.Path("browser-parity"))
    ap.add_argument("--browsers", nargs="+", default=["firefox", "chromium", "webkit"])
    ap.add_argument('--surfaces',nargs='+',choices=list(SURFACES),default=[k for k in SURFACES if k not in ("map-river","river","city-food")])
    ap.add_argument('--rich-user',help='additional content-stress city account')
    a = ap.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    token = login(USER)
    try:
        host_token = login(HOST_USER)
    except urllib.error.HTTPError:
        sys.exit(f"ingen spelare {HOST_USER} — skapa en utan stad: tools/acceptance.sh player {HOST_USER}")
    rows = []
    rich_token=login(a.rich_user) if a.rich_user else None
    surfaces={k:SURFACES[k] for k in a.surfaces}
    with sync_playwright() as pw:
        for engine in a.browsers:
            rows += run_engine(pw, engine, token, a.out, surfaces)
            rows += run_engine(pw, engine, host_token, a.out, HOST_SURFACES,close_drawers=False)
            if rich_token:rows += run_engine(pw,engine,rich_token,a.out,RICH_SURFACES,close_drawers=False)
            (a.out / "measurements.json").write_text(json.dumps(rows,indent=1,ensure_ascii=False))
            print(engine,'complete',len(rows),'measurements',flush=True)
    (a.out / "measurements.json").write_text(json.dumps(rows, indent=1, ensure_ascii=False))
    f = findings(rows)
    (a.out / "findings.txt").write_text("\n".join(f) + "\n")
    print("\n".join(f) if f else "inga fynd")
    print(f"\n{len(rows)} mätningar, {len(f)} fynd → {a.out}")
    sys.exit(1 if f else 0)


if __name__ == "__main__":
    main()
