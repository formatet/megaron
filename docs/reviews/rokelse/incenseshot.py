import json, pathlib, sys, urllib.request
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright
HOST="http://10.0.1.92:8080"
WT=pathlib.Path("/home/tk/Projects/megaron/.claude/worktrees/agent-a5c6db024397d7605/web/static")
OUT=pathlib.Path(sys.argv[1]); OUT.mkdir(parents=True, exist_ok=True)
INC={(58,57),(59,56),(57,58)}  # scrub_maquis hexes Eurymede can see, near the tin deposits
ORIG="""        ctx.fillStyle = '#D8A838';
        ctx.beginPath(); ctx.moveTo(ox, oy - 4.5); ctx.lineTo(ox + 3, oy); ctx.arc(ox, oy + 0.5, 3, 0, Math.PI); ctx.lineTo(ox - 3, oy); ctx.closePath(); ctx.fill(); ctx.stroke();"""
DROP="ctx.beginPath(); ctx.moveTo(ox, oy - 4.5); ctx.lineTo(ox + 3, oy); ctx.arc(ox, oy + 0.5, 3, 0, Math.PI); ctx.lineTo(ox - 3, oy); ctx.closePath();"
V0={
 "B1":ORIG,
 "B2":"ctx.fillStyle='#F2C230'; "+DROP+" ctx.fill(); ctx.stroke(); ctx.fillStyle='#FFF6C8'; ctx.fillRect(Math.round(ox-1.5),Math.round(oy-1),1,2);",
 "B3":"ctx.save(); ctx.fillStyle='#FFE680'; ctx.beginPath(); ctx.moveTo(ox, oy - 6.5); ctx.lineTo(ox + 4.5, oy); ctx.arc(ox, oy + 0.5, 4.5, 0, Math.PI); ctx.lineTo(ox - 4.5, oy); ctx.closePath(); ctx.fill(); ctx.restore(); ctx.fillStyle='#F2C230'; "+DROP+" ctx.fill(); ctx.stroke(); ctx.fillStyle='#FFF6C8'; ctx.fillRect(Math.round(ox-1.5),Math.round(oy-1),1,2);",
 "B4":"ctx.fillStyle='#F2C230'; "+DROP+" ctx.fill(); ctx.stroke(); ctx.fillStyle='#FFF6C8'; ctx.fillRect(Math.round(ox-1.5),Math.round(oy-1),1,2); ctx.fillStyle='#FFE680'; [[5,-5],[-5,-3],[5,3]].forEach(([dx,dy])=>{const x=Math.round(ox+dx),y=Math.round(oy+dy); ctx.fillRect(x,y-1,1,3); ctx.fillRect(x-1,y,3,1);});",
}
V=V0
cfg=json.load(open(pathlib.Path.home()/".config/megaron-ai/luna-eurymede.json"))
a=json.load(urllib.request.urlopen(urllib.request.Request(HOST+"/api/v1/auth/login",json.dumps({"username_or_email":cfg["username"],"password":cfg["password"]}).encode(),{"Content-Type":"application/json"})))
tok=a.get("access_token") or a["token"]
for code,snip in V.items():
  def mk(snip):
   def route(r):
    return _route(r, snip)
   return route
  def _route(r, snip):
    path=urlparse(r.request.url).path
    if path.startswith("/static/"):
      f=WT/path[len("/static/"):]
      if f.is_file() and f.suffix in(".js",".css"):
        body=f.read_text()
        if path.endswith("/render/map.js"):
          assert ORIG in body; body=body.replace(ORIG,"        "+snip)
        return r.fulfill(body=body,headers={"Content-Type":"text/css" if f.suffix==".css" else "application/javascript","Cache-Control":"no-store"})
    if path.endswith("/map") and "/api/v1/worlds/" in path:
      resp=r.fetch(); d=resp.json()
      for t in d:
        if (t["q"],t["r"]) in INC: t["incense_deposit"]=True; print("MARK",t["q"],t["r"],t.get("terrain"),t.get("tier"),t.get("tin_deposit"))
      return r.fulfill(response=resp, body=json.dumps(d), headers={"Content-Type":"application/json"})
    return r.continue_()
  with sync_playwright() as p:
    b=p.firefox.launch(); pg=b.new_page(viewport={"width":1100,"height":800}); pg.on("pageerror", lambda e: print("PAGEERR", e)); pg.on("requestfailed", lambda q: print("FAIL", q.url))
    rt=mk(snip); pg.route("**/static/**", rt); pg.route("**/api/v1/worlds/*/map", rt)
    pg.goto(HOST+"/",wait_until="domcontentloaded")
    pg.evaluate("t=>localStorage.setItem('poleia_token',t)",tok)
    pg.context.add_cookies([{"name":"poleia_token","value":tok,"url":HOST}])
    pg.goto(HOST+"/play",wait_until="load")
    pg.wait_for_function("()=>window.centreOn!==undefined",timeout=20000)
    pg.evaluate("()=>window.centreOn(58,57)"); pg.wait_for_timeout(2500)
    print(code, pg.evaluate("()=>{const S=window.State||{}; const td=S.tileData; return {has:!!td, n: td? (Array.isArray(td)?td:Object.values(td)).filter(t=>t&&t.incense_deposit).length : -1, keys:Object.keys(window).filter(k=>/state/i.test(k)).slice(0,5)}}"))
    pg.screenshot(path=str(OUT/f"{code}-full.png")); pg.screenshot(path=str(OUT/f"{code}.png"),clip={"x":540,"y":560,"width":260,"height":200})
    b.close()
  print(code,"ok")
