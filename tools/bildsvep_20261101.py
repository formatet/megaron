#!/usr/bin/env python3
"""BILD-svep 2026-11-01: own isolated runtime (docker postgres+redis, built server) and native Firefox.
Usage: python3 tools/bildsvep_20261101.py OUTDIR [--keep STATE.json] [--scenes A1,A2]
  --keep STATE.json : dev mode; leaves containers+server running and reuses them on the next call.
Every scene fixture (SQL) is appended to OUTDIR/fixture.sql under a '-- scene' header.
No production source is touched; no shared rig. Cleanup happens unless --keep.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,sys,argparse
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
ap=argparse.ArgumentParser();ap.add_argument('out');ap.add_argument('--keep');ap.add_argument('--scenes');ap.add_argument('--down',action='store_true');ap.add_argument('--index',action='store_true')
A=ap.parse_args()
OUT=Path(A.out).resolve();OUT.mkdir(parents=True,exist_ok=True)
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
S={}  # state
containers=[];proc=None
FIX=[];SHOTS=[];PROBLEMS=[]
def launch(kind,image,args):
 name='megaron-bildsvep-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return name,json.loads(run('docker',"inspect",name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
def cleanup():
 for n in S.get('containers',[])+containers:subprocess.run(['docker','rm','-fv',n],capture_output=True)
 if S.get('pid'):
  try:os.kill(S['pid'],15)
  except OSError:pass
def api(path,method='GET',data=None,token=None,raw=False):
 req=urllib.request.Request(S['base']+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
 try:r=urllib.request.urlopen(req,timeout=30)
 except urllib.error.HTTPError as e:
  if raw:return {'status':e.code,'body':e.read().decode()}
  raise RuntimeError(f'{method} {path} -> {e.code} {e.read().decode()[:300]}')
 return json.load(r)
CUR=['setup']
def sql(q,fixture=False):
 if fixture:
  FIX.append(f'-- scene {CUR[0]}\n{q}')
  with (OUT/'fixture.log.sql').open('a') as f:f.write(f'-- scene {CUR[0]}\n{q}\n\n')
 return run('docker','exec',S['pgc'],'psql','-U','postgres','-d','bildsvep','-At','-v','ON_ERROR_STOP=1','-c',q)
def setup():
 global proc
 tmp=Path(tempfile.mkdtemp(prefix='bildsvep-'));S['tmp']=str(tmp)
 commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD');S['commit']=commit
 run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
 pgc,pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=bildsvep','-e','POSTGRES_DB=bildsvep']);rc,redis=launch('redis','redis:7-alpine',[])
 S['pgc']=pgc;S['containers']=list(containers)
 for _ in range(100):
  if subprocess.run(['docker','exec',pgc,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 time.sleep(1)
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 S['base']=f'http://127.0.0.1:{port}'
 env={**clean,'DATABASE_URL':f'postgres://postgres:bildsvep@localhost:{pg}/bildsvep?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Bildsvep','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
 p=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT,start_new_session=True);S['pid']=p.pid
 for _ in range(160):
  try:h=api('/healthz');break
  except Exception:
   if p.poll() is not None:raise RuntimeError('server exited')
   time.sleep(.5)
 S['health']=h;assert h['commit']==commit,h
 S['world']=api('/api/v1/worlds')[0]['id']
 json.dump(S,open(A.keep or str(tmp/'state.json'),'w'))
def actor(name,city):
 wp='/api/v1/worlds/'+S['world']
 token=api('/api/v1/auth/register','POST',{'username':name.lower()+'-'+secrets.token_hex(3),'password':secrets.token_urlsafe(30)})['access_token']
 api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
 host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
 sql(f"UPDATE players SET wanax_name='{name}' WHERE id='{pid}'",True)
 return dict(name=name,city=city,token=token,pid=pid,host=host['id'])
def found(name,city,x):
 W=S['world'];wp='/api/v1/worlds/'+W;r=S['r']
 who=actor(name,city)
 sql(f"UPDATE units SET q={x},r={r},status='positioned' WHERE id='{who['host']}';",True)
 who['sid']=api(wp+'/founding/settle','POST',{'name':city},who['token'])['settlement_id'];who['x']=x
 sql(f"UPDATE units SET q={x},r={r},status='garrison',settlement_id='{who['sid']}',support_settlement_id='{who['sid']}' WHERE world_id='{W}' AND owner_id='{who['pid']}' AND type='spearman';",True)
 who['province']=next(p['id'] for p in api(wp+'/provinces',token=who['token']) if p.get('settlement_id')==who['sid'])
 S['actors'][name]=who
 return who
def world_scene():
 """Cities on one prepared plains row; coastal water along the south edge."""
 W=S['world']
 site=json.loads(sql(f"SELECT row_to_json(p) FROM (SELECT a.q,a.r FROM map_tiles a WHERE a.world_id='{W}' AND a.terrain='plains' AND a.q BETWEEN 4 AND 20 AND a.r BETWEEN 6 AND 30 ORDER BY a.r,a.q LIMIT 1)p"))
 q,r=site['q'],site['r'];S['q'],S['r']=q,r
 CUR[0]='world'
 sql(f"UPDATE map_tiles SET terrain='plains',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='{W}' AND r BETWEEN {r-3} AND {r} AND q BETWEEN {q-3} AND {q+30}; UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='{W}' AND r BETWEEN {r+1} AND {r+3} AND q BETWEEN {q-3} AND {q+30}; UPDATE map_tiles SET coastal=true WHERE world_id='{W}' AND r={r} AND q BETWEEN {q-3} AND {q+30}; UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='{W}' AND r BETWEEN {r-3} AND {r+3} AND q BETWEEN {q-3} AND {q+30};",True)
 S['actors']={}
 for name,city,x in [('Idomeneus','Phaistos',q),('Myrine','Gournia',q+6),('Nestor','Pylos',q+14),('Atreus','Kydonia',q+22),('Minos','Knossos',q+28)]:found(name,city,x)
 sql(f"UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{W}'",True)
 sql(f"UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id='{S['actors']['Idomeneus']['sid']}'",True)
 json.dump(S,open(A.keep or str(Path(S['tmp'])/'state.json'),'w'))

SCENES={}   # code -> (title, what to judge)
def scene(code,title,judge):
 def deco(f):SCENES[code]=(title,judge,f);return f
 return deco
def me():return S['actors']['Idomeneus']
def newpage(br,who,w,init=None):
 a=S['actors'][who] if isinstance(who,str) else who
 h=900 if w>600 else 844
 ctx=br.new_context(viewport={'width':w,'height':h});ctx.add_cookies([{'name':'poleia_token','value':a['token'],'url':S['base']}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(a['token'])+')')
 if init:ctx.add_init_script(init)
 pg=ctx.new_page();pg.errs=[]
 pg.on('pageerror',lambda e:(pg.errs.append(str(e)),PROBLEMS.append(f'{CUR[0]}@{w}: pageerror {e}')))
 pg.on('console',lambda m:m.type=='error' and PROBLEMS.append(f'{CUR[0]}@{w}: console.error {m.text[:200]}'))
 pg.goto(S['base']+'/world/'+S['world']+'/map',wait_until='networkidle');pg.wait_for_timeout(1200)
 try:
  if pg.locator('#dc-dismiss-all').is_visible():pg.locator('#dc-dismiss-all').click()
  for b in pg.locator('.drawer.open .lb-dismiss').all():
   if b.is_visible():b.click()
 except Exception:pass
 pg.wait_for_timeout(300)
 return pg
def jsmod(pg,mod,expr,arg=None):
 return pg.evaluate("async a=>{const m=await import('/static/js/megaron/"+mod+"');return ("+expr+")}",arg)
def open_hex(pg,q,r):
 pg.evaluate("async a=>{const m=await import('/static/js/megaron/render/map.js');m.focusDispatchHex(a.q,a.r)}",{'q':q,'r':r});pg.wait_for_timeout(700)
def shot(pg,code,tag,note,sel=None,pad=8,full=False,tall=False,maxh=None):
 """sel: css selector (or list) -> crop to its box; None -> viewport. width taken from viewport."""
 w=pg.viewport_size['width'];h0=pg.viewport_size['height']
 if tall:pg.set_viewport_size({'width':w,'height':1500});pg.wait_for_timeout(500)
 pg.wait_for_timeout(300);name=f'{code}-{w}-{tag}.png';path=OUT/name
 if sel:
  box=pg.locator(sel).first.bounding_box()
  vh=pg.viewport_size['height']
  x=max(0,box['x']-pad);y=max(0,box['y']-pad)
  clip={'x':x,'y':y,'width':min(w-x,box['width']+2*pad),'height':min(vh-y,box['height']+2*pad)}
  if maxh:clip['height']=min(clip['height'],maxh)
  pg.screenshot(path=str(path),clip=clip)
 else:pg.screenshot(path=str(path),full_page=full)
 if tall:pg.set_viewport_size({'width':w,'height':h0});pg.wait_for_timeout(300)
 SHOTS.append({'code':code,'file':name,'width':w,'tag':tag,'note':note})
 (OUT/'shots.json').write_text(json.dumps(SHOTS,indent=1))
def run_scenes(codes):
 with sync_playwright() as pw:
  br=pw.firefox.launch()
  for c in codes:
   CUR[0]=c;SHOTS[:]=[x for x in SHOTS if not x['code'].startswith(c+'.') and x['code']!=c]
   try:SCENES[c][2](br)
   except Exception as e:
    import traceback;traceback.print_exc();PROBLEMS.append(f'{c}: scene failed: {e!r}')
  br.close()

def fx_foreign():
 q,r=S['q'],S['r'];A=S['actors'];W=S['world']
 g,py,ky=A['Myrine'],A['Nestor'],A['Atreus']
 # live: one of Idomeneus's spearmen stands next to Gournia
 sql(f"UPDATE units SET q={g['x']-2},r={r},status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='{W}' AND owner_id='{me()['pid']}' AND type='spearman' LIMIT 1);",True)
 # memory: Pylos seen once at tick 40 (small), tile remembered
 sql(f"INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('{W}','{me()['pid']}','{py['sid']}',{py['x']},{r},'Pylos','{py['pid']}','Nestor',true,'active',0,380,40) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('{W}','{me()['pid']}','{py['province']}') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '{W}','{me()['pid']}',q,r FROM map_tiles WHERE world_id='{W}' AND (abs(q-{py['x']})+abs(q+r-{py['x']}-{r})+abs(r-{r}))/2<=2 ON CONFLICT DO NOTHING;",True)
 sql(f"INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('{g['sid']}','{W}','{{tin,timber}}','{{wine,pottery}}',80) ON CONFLICT DO NOTHING; INSERT INTO market_board_readings(world_id,player_id,settlement_id,buys,sells,written_tick,read_tick) VALUES('{W}','{me()['pid']}','{g['sid']}','{{tin,timber}}','{{wine,pottery}}',80,95) ON CONFLICT DO NOTHING;",True)
 # contact: a messenger of Idomeneus read Kydonia at tick 70
 sql(f"INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('{W}','{me()['pid']}','{ky['sid']}',{ky['x']},{r},'Kydonia','{ky['pid']}','Atreus',true,'active',0,1500,70) ON CONFLICT DO NOTHING; INSERT INTO player_contacted_settlements(world_id,player_id,settlement_id,contact_tick,source,name,owner_id,owner_name,size,food,goods,buildings) VALUES('{W}','{me()['pid']}','{ky['sid']}',70,'messenger','Kydonia','{ky['pid']}','Atreus','town','well fed','[{{\"word\":\"plenty of\",\"good\":\"olive oil\"}},{{\"word\":\"some\",\"good\":\"timber\"}}]','[{{\"type\":\"granary\",\"level\":2}},{{\"type\":\"harbour\",\"level\":1}}]') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('{W}','{me()['pid']}','{ky['province']}') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '{W}','{me()['pid']}',q,r FROM map_tiles WHERE world_id='{W}' AND (abs(q-{ky['x']})+abs(q+r-{ky['x']}-{r})+abs(r-{r}))/2<=1 ON CONFLICT DO NOTHING;",True)

@scene('A1','Foreign city panel: live / memory / contact','Three states of one panel. Live: "Seen now"; memory: "Seen <date>" with the old size; contact: "Your messenger was there ..." with coarse words and "Buildings then". Does each line say how it is known, and is nothing clipped?')
def A1(br):
 fx_foreign()
 q,r=S['q'],S['r'];A=S['actors'];g,py,ky=A['Myrine'],A['Nestor'],A['Atreus']
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w)
  for who,tag in ((g,'live'),(py,'memory'),(ky,'contact')):
   open_hex(pg,who['x'],r);pg.wait_for_timeout(1200)
   if w==1280 and tag=='live':shot(pg,'A1','full-live','Whole window, Gournia selected (live sight)')
   shot(pg,'A1',tag,{'live':'Gournia, in sight now','memory':'Pylos, remembered (seen tick 40)','contact':'Kydonia, only a messenger snapshot'}[tag],'#inspect-panel')
  pg.context.close()

def deliver_letter(frm,to,message,**extra):
 """Send a messenger from frm's city to to's city and make the real arrival worker deliver it now."""
 W=S['world'];wp='/api/v1/worlds/'+W
 m=api(wp+'/settlements/'+frm['sid']+'/messengers','POST',{'destination_id':to['sid'],'message':message,**extra},frm['token'])
 sql(f"UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='{W}' AND event_type LIKE 'MessengerArrival%' AND payload->>'messenger_id'='{m['id']}';",True)
 for _ in range(150):
  if sql(f"SELECT status FROM messengers WHERE id='{m['id']}'")=='delivered':return m
  time.sleep(.2)
 raise RuntimeError('arrival worker did not deliver')
def drawer(pg,name):
 pg.locator('#trig-'+name).click();pg.wait_for_timeout(600)
@scene('A2','Counter-offer on a trade letter (K6)','Counter-offer button beside Accept/Decline; the inline row (Buy/Sell qty for silver, Send/Cancel) and the sent receipt. Is it clear that this is a new offer, not an acceptance?')
def A2(br):
 g=S['actors']['Myrine'];W=S['world']
 sql(f"UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('{g['sid']}','{me()['sid']}')",True)
 for w in (1280,390):
  deliver_letter(g,me(),'Idomeneus, I will pay well for your timber.'+(' ' if w==1280 else '  '),trade_offer={'kind':'buy','want_good':'timber','want_qty':5,'offer_silver':15})
  pg=newpage(br,'Idomeneus',w);drawer(pg,'diplomacy')
  pg.locator('#drawer-diplomacy').get_by_text('Idomeneus, I will pay well').last.click();pg.wait_for_timeout(800)
  shot(pg,'A2','offer','The incoming offer in the thread, with Accept / Counter-offer / Decline','#drawer-diplomacy')
  pg.get_by_role('button',name='Counter-offer').first.click();pg.wait_for_timeout(400)
  pg.locator('[id^=dip-counter-qty-]').first.fill('4');pg.locator('[id^=dip-counter-silver-]').first.fill('18')
  shot(pg,'A2','counter-row','Counter-offer row opened and filled in (sell 4 timber for 18 silver)','#drawer-diplomacy')
  if True:
   pg.get_by_role('button',name='Send',exact=True).first.click();pg.wait_for_timeout(1500)
   shot(pg,'A2','sent','After Send: the receipt line (arrival time, "They must agree to the new terms")','#drawer-diplomacy')
  pg.context.close()

def fx_city():
 """Idomeneus's Phaistos: plausible stocks (grain/fish/oil with caps), a few buildings, a market board."""
 W=S['world'];sid=me()['sid'];pid=me()['pid']
 sql(f"UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='{sid}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','market',1),('{sid}','barracks',1),('{sid}','stable',1),('{sid}','silo',1),('{sid}','pen',1),('{sid}','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='{me()['province']}'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('{sid}','{W}','{{tin,copper}}','{{pottery,oil}}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '{W}','{pid}',q,r FROM map_tiles WHERE world_id='{W}' AND (abs(q-{me()['x']})+abs(q+r-{me()['x']}-{S['r']})+abs(r-{S['r']}))/2<=5 ON CONFLICT DO NOTHING;",True)
def closeall(pg):
 pg.evaluate("document.querySelectorAll('.drawer.open').forEach(d=>window.toggleDrawer(d.id.replace('drawer-','')))");pg.wait_for_timeout(300)
@scene('A6','City: food row and goods without wine','Production tab: the food rows (grain, fish, oil, herd) and net per game day. Wine is a good but not food: it must not appear among the food rows.')
def A6(br):
 fx_city()
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  shot(pg,'A6','full','City drawer, Production tab (whole window)')
  shot(pg,'A6','production','Production tab: food rows','#drawer-city',tall=True)
  pg.locator('#drawer-city [data-tab="forrad"]').click();pg.wait_for_timeout(500)
  shot(pg,'A6','goods','Warehouse tab: goods list (wine listed as an ordinary good, not food)','#drawer-city',tall=True)
  pg.context.close()

def fx_cult():
 W=S['world'];sid=me()['sid'];pid=me()['pid']
 sql(f"INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{sid}','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=62,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='{W}' AND player_id='{pid}';",True)
@scene('A9','Kult: Kharis / Prayers / Temples','The three tabs of the Kult drawer. Kharis: the heading "Kharis each game day" with short effect rows. Prayers and Temples: are the short rows quick to read, nothing cut off?')
def A9(br):
 fx_city();fx_cult()
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'kult')
  shot(pg,'A9','full','Kult drawer on open (whole window)')
  tabs=pg.locator('#drawer-kult .dtab')
  for i in range(tabs.count()):
   t=tabs.nth(i);name=t.inner_text().strip().lower();t.click();pg.wait_for_timeout(700)
   shot(pg,'A9',name,f'Kult drawer, tab {name}','#drawer-kult',tall=True)
  pg.context.close()
@scene('A17','City: Warehouse tab and Garrison -> Recruit','Warehouse tab (food for N game days, stores with caps, "full" marks) and the Recruit button beside the Garrison title; the click opens War on the Recruit tab for this city.')
def A17(br):
 fx_city()
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  pg.locator('#drawer-city [data-tab="forrad"]').click();pg.wait_for_timeout(500)
  shot(pg,'A17','warehouse','Warehouse tab','#drawer-city',tall=True)
  pg.locator('#drawer-city [data-tab="garnison"]').click();pg.wait_for_timeout(600)
  shot(pg,'A17','garrison','Garrison tab with the Recruit button beside the title','#drawer-city',tall=True)
  pg.get_by_role('button',name='Recruit').first.click();pg.wait_for_timeout(900)
  shot(pg,'A17','recruit','After clicking Recruit: War drawer on the Recruit tab, this city selected','#drawer-war',tall=True)
  pg.context.close()
@scene('A8','Forces (War drawer)','Forces tab (spelarordet Forces instead of Units), tab labels Forces / Recruit / Marches, per-row state. Read the rows as a player: is it clear what each force is doing?')
def A8(br):
 fx_city()
 W=S['world'];sid=me()['sid'];pid=me()['pid']
 sql(f"INSERT INTO units(world_id,owner_id,type,size,q,r,status,settlement_id,support_settlement_id) SELECT '{W}','{pid}','chariot',6,q,r,'garrison',settlement_id,support_settlement_id FROM units WHERE owner_id='{pid}' AND type='spearman' LIMIT 1;",True) if False else None
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'war')
  shot(pg,'A8','forces','War drawer, Forces tab','#drawer-war',tall=True)
  pg.locator('#drawer-war [data-tab="recruit"]').click();pg.wait_for_timeout(500)
  shot(pg,'A8','recruit','Recruit tab','#drawer-war',tall=True)
  pg.locator('#drawer-war [data-tab="movements"]').click();pg.wait_for_timeout(500)
  shot(pg,'A8','marches','Marches tab','#drawer-war',tall=True)
  pg.context.close()

def hex_screen(pg,q,r):
 return pg.evaluate("""async a=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');
  const p=m.hexPx(a.q,a.r),c=m.canvas.getBoundingClientRect();return {x:c.left+State.camera.x+p.x*m.SCALE*State.camera.zoom,y:c.top+State.camera.y+p.y*m.SCALE*State.camera.zoom}}""",{'q':q,'r':r})
def centre(pg,q,r):
 pg.evaluate("a=>window.centreOn(a.q,a.r)",{'q':q,'r':r});pg.wait_for_timeout(600)
@scene('A20','Map hover: a named place shows its hex coordinates','Hover tooltip over a city: "Gournia (18,21) · Plains · Wanax: Myrine" and, for a remembered city, the same. Is the coordinate pair easy to read, and does the tooltip fit on one line at 390?')
def A20(br):
 fx_foreign();fx_city();A=S['actors'];r=S['r']
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w)
  for who,tag in ((A['Myrine'],'live'),(A['Nestor'],'memory')):
   centre(pg,who['x'],r);pt=hex_screen(pg,who['x'],r)
   pg.mouse.move(pt['x']-60,pt['y']-40);pg.wait_for_timeout(200);pg.mouse.move(pt['x'],pt['y']);pg.wait_for_timeout(700)
   shot(pg,'A20',tag,f"Hover over {who['city']}: tooltip with coordinates (whole window)")
  pg.context.close()
@scene('A7','Map edge beside an open drawer','Open a drawer and pan the map as far east as it goes: the right edge of the world should come to rest at the drawer\'s left edge instead of hiding under it. Judge the rim look too (dark band / hatch).')
def A7(br):
 fx_city()
 for w in (1280,):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'kult')
  pg.evaluate("async()=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');State.camera.x=-99999;m.clampCamera();State.dirty=true}");pg.wait_for_timeout(800)
  shot(pg,'A7','east','Panned fully east with the Kult drawer open')
  pg.evaluate("async()=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');State.camera.x=99999;m.clampCamera();State.dirty=true}");pg.wait_for_timeout(800)
  shot(pg,'A7','west','Panned fully west (same drawer open) for comparison')
  pg.context.close()
@scene('A18','Top bar: "Megaron was updated - Reload"','The notice that appears when the server version changes under an open tab. Is it noticeable but not alarming, and does it fit at 390?')
def A18(br):
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w)
  def bump(route):
   resp=route.fetch();route.fulfill(response=resp,headers={**resp.headers,'x-megaron-version':'deadbeef-new'})
  pg.route('**/api/**',bump)
  pg.wait_for_selector('#update-notice.visible',timeout=30000)
  shot(pg,'A18','notice','After an API answer carrying a different X-Megaron-Version (injected by the proof rig via route); whole window')
  pg.screenshot(path=str(OUT/f'A18-{w}-crop.png'),clip={'x':0,'y':0,'width':w,'height':46});SHOTS.append({'code':'A18','file':f'A18-{w}-crop.png','width':w,'tag':'crop','note':'Top bar only, 1:1'});(OUT/'shots.json').write_text(json.dumps(SHOTS,indent=1))
  pg.unroute_all(behavior='ignoreErrors')
  pg.context.close()
@scene('A11','Discord icon (first proposal)','The icon as committed in docs/branding (downscaled copy here). Does it hold up at small sizes?')
def A11(br):
 from PIL import Image
 im=Image.open(ROOT/'docs/branding/megaron-discord-icon-v1.png').convert('RGBA');print('discord icon',im.size)
 big=im.resize((512,512),Image.LANCZOS);big.save(OUT/'A11-1280-icon-512.png',optimize=True)
 small=Image.new('RGBA',(300,140),(240,232,212,255))
 x=10
 for sz in (128,64,32,16):
  small.paste(im.resize((sz,sz),Image.LANCZOS),(x,10),im.resize((sz,sz),Image.LANCZOS));x+=sz+12
 small.save(OUT/'A11-1280-icon-small.png',optimize=True)
 SHOTS.extend([{'code':'A11','file':'A11-1280-icon-512.png','width':1280,'tag':'icon','note':'Icon downscaled to 512 px (source is %dx%d, 1.1 MB)'%im.size},{'code':'A11','file':'A11-1280-icon-small.png','width':1280,'tag':'sizes','note':'128, 64, 32 and 16 px on the page background (Discord shows it around 32-128)'}])
 (OUT/'shots.json').write_text(json.dumps(SHOTS,indent=1))

def places_of(who):
 return api('/api/v1/worlds/'+S['world']+'/places',token=S['actors'][who]['token'])['places']
PLACE_NAMES={'island':'Dia','strait':'Euripos','cape':'Sounion','plain':'Mesara','hills':'Ida','delta':'Axios Mouths','lake':'Kopais','river':'Eurotas','massif':'Olympos','forest':'Pindos','desert':'Libya'}
def fx_geography():
 """Minos sees the whole map; one place of each kind is named (first-come claim by Minos)."""
 W=S['world'];m=S['actors']['Minos']
 sql(f"INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'{m['pid']}',q,r FROM map_tiles WHERE world_id='{W}' ON CONFLICT DO NOTHING",True)
 ps=places_of('Minos');done=set()
 rows=[]
 for p in ps:
  if p['kind'] in done or p['kind'] not in PLACE_NAMES:continue
  if p['kind'] in('plain',) and len(p['hexes'])>60:continue
  done.add(p['kind']);a,b=p['id'].split(',')
  rows.append(f"('{W}','{p['kind']}',{a},{b},'{m['pid']}',100,'{PLACE_NAMES[p['kind']]}',100)")
 sql("INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick,name,named_tick) VALUES "+','.join(rows)+" ON CONFLICT (world_id,kind,anchor_q,anchor_r) DO UPDATE SET name=EXCLUDED.name,claimed_by=EXCLUDED.claimed_by,named_tick=100",True)
 return {p['kind']:p for p in places_of('Minos') if p['name']}
@scene('A19','Place labels on the map: island / strait / cape / plain / hills / delta','Named places drawn on the map. Does each kind read as a place name (not a city) and does the label stay legible over its terrain? One crop per kind, then the full window for one.')
def A19(br):
 named=fx_geography()
 for w in (1280,390):
  pg=newpage(br,'Minos',w)
  for kind in ('island','strait','cape','plain','hills','delta'):
   p=named.get(kind)
   if not p:PROBLEMS.append(f'A19: no {kind} place in this map');continue
   hx=p['hexes'][len(p['hexes'])//2];centre(pg,hx['q'],hx['r'])
   shot(pg,'A19',kind,f"{p['name']} ({kind}), {len(p['hexes'])} hexes; window centred on one of its hexes")
  pg.context.close()

def pick_river():
 """Longest river place on this map: (hexes ordered source->mouth, delta hex or None)."""
 W=S['world']
 rivers=[p for p in places_of('Minos') if p['kind']=='river']
 rivers.sort(key=lambda p:-len(p['hexes']));best=rivers[0]['hexes']
 rows={(int(a),int(b)):int(u) for a,b,u in (x.split('|') for x in sql(f"SELECT q,r,COALESCE(river_upstream_edges,0) FROM map_tiles WHERE world_id='{W}' AND terrain IN('river','river_ford')").splitlines())}
 hs=[(h['q'],h['r']) for h in best if (h['q'],h['r']) in rows]
 # order: start at a hex with no upstream edge, walk by adjacency
 nb=[(1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1)]
 start=next((h for h in hs if rows[h]==0),hs[0]);order=[start];left=set(hs)-{start}
 while left:
  cur=order[-1];nxt=next((n for n in ((cur[0]+d[0],cur[1]+d[1]) for d in nb) if n in left),None)
  if not nxt:break
  order.append(nxt);left.discard(nxt)
 delta=None
 for d in nb:
  n=(order[-1][0]+d[0],order[-1][1]+d[1])
  if sql(f"SELECT 1 FROM map_tiles WHERE world_id='{W}' AND q={n[0]} AND r={n[1]} AND terrain='river_delta'"):delta=n
 return order,delta
@scene('A13','River on the map: flow direction and fog edge','Rivers as drawn, with the flow chevrons; the hex panel says "Flows from ... toward ..." and, at the edge of what you know, "Continues into the fog toward ...". Judge: do the chevrons read as direction, and does the river end sensibly at source, mouth and fog?')
def A13(br):
 fx_city();order,delta=pick_river();W=S['world'];i=me()
 mid=order[len(order)//2]
 half=order[:max(2,len(order)//2)]
 sql("INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '%s','%s',q,r FROM map_tiles WHERE world_id='%s' AND (%s) ON CONFLICT DO NOTHING"%(W,i['pid'],W,' OR '.join(f"(abs(q-{a})+abs(q+r-{a}-{b})+abs(r-{b}))/2<=1" for a,b in half)),True)
 stops=[(order[0],'source','Source hex panel'),(mid,'middle','Middle hex panel')]+([(delta,'mouth','Mouth (delta) hex panel')] if delta else [])
 for w in (1280,390):
  pg=newpage(br,'Minos',w);centre(pg,*mid)
  shot(pg,'A13','map',f'River of {len(order)} hexes, source {order[0]}, mouth {delta}; centred on {mid} (Minos sees the whole map)')
  for (q,r),tag,note in stops:
   open_hex(pg,q,r);shot(pg,'A13',tag,note+f' ({q},{r})','#inspect-panel')
  pg.context.close()
  pg=newpage(br,'Idomeneus',w);centre(pg,*half[-1])
  shot(pg,'A13','fog-map',"Same river for Idomeneus, who has scouted only its upper half: it runs into the fog")
  open_hex(pg,*half[-1]);shot(pg,'A13','fog-panel',f'Panel of the last known river hex {half[-1]}','#inspect-panel')
  pg.context.close()
@scene('A14','Lake on the map','A lake is drawn like coastal water. Timothy decides whether it needs its own tone. Shown: lake next to ordinary coastal sea and the hex panel/label.')
def A14(br):
 fx_geography()
 lk=sql(f"SELECT q,r FROM map_tiles WHERE world_id='{S['world']}' AND terrain='lake' ORDER BY q,r").splitlines()
 big=[p for p in places_of('Minos') if p['kind']=='lake']
 big.sort(key=lambda p:-len(p['hexes']))
 for w in (1280,390):
  pg=newpage(br,'Minos',w)
  for k,p in enumerate(big[:2]):
   hx=p['hexes'][0];centre(pg,hx['q'],hx['r'])
   shot(pg,'A14',f'lake{k+1}',f"Lake {p['id']} ({len(p['hexes'])} hexes){' named '+p['name'] if p['name'] else ''}")
  hx=big[0]['hexes'][0];open_hex(pg,hx['q'],hx['r']);shot(pg,'A14','panel','Hex panel on a lake hex','#inspect-panel')
  pg.context.close()

@scene('A4','Remembered city: "as last seen ..."','The Owner row of a city seen earlier but not now: "Nestor - as last seen yesterday 07:00", Defence "unknown", and below it "Seen <date>: hamlet" (the old size, not today\'s). Does it read as memory, not as the current state?')
def A4(br):
 fx_foreign();py=S['actors']['Nestor']
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_hex(pg,py['x'],S['r']);pg.wait_for_timeout(1000)
  shot(pg,'A4','owner','Pylos panel, the upper rows (Owner / Defence)','#inspect-panel',maxh=260)
  pg.hover('#inspect-panel');pg.context.close()
@scene('A5','Market board: own (Buildings tab) and a foreign one as read','Buildings tab with the market: "Market board" section where the ruler writes what the city buys and sells. And on a foreign city (Gournia): "Market board - Buys tin, timber - Sells pottery, wine" with when it was read and how many game days ago.')
def A5(br):
 fx_foreign();fx_city();g=S['actors']['Myrine']
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  pg.locator('#drawer-city [data-tab="byggnader"]').click();pg.wait_for_timeout(700)
  shot(pg,'A5','buildings','City drawer, Buildings tab','#drawer-city',tall=True)
  pg.locator('#drawer-city [data-tab="byggnader"]').click()
  pg.evaluate("window.toggleDrawer('city')");pg.wait_for_timeout(300)
  open_hex(pg,g['x'],S['r']);pg.wait_for_timeout(1200)
  shot(pg,'A5','foreign','Gournia panel: the board as Idomeneus last read it','#inspect-panel')
  pg.context.close()
@scene('A3','Tell of a place / Told by','Writing to another ruler: the "Tell of a place" selector in the message form, then on the receiving side the place known only from the story: "Told by Idomeneus - as they saw it, not seen by your own people". Is the difference between told and seen clear?')
def A3(br):
 fx_foreign();fx_city();W=S['world'];i=me();g=S['actors']['Myrine']
 mine=[p for p in places_of('Idomeneus') if p['kind'] in('river','plain','hills','cape','island','delta','lake','strait') and p['hexes']]
 pl=mine[0];a,b=pl['id'].split(',')
 sql(f"INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick,name,named_tick) VALUES('{W}','{pl['kind']}',{a},{b},'{i['pid']}',90,'Alpheios',90) ON CONFLICT (world_id,kind,anchor_q,anchor_r) DO UPDATE SET name='Alpheios',claimed_by='{i['pid']}'",True)
 hx=pl['hexes'][0]
 pg=newpage(br,'Idomeneus',1280);open_hex(pg,g['x'],S['r']);pg.wait_for_timeout(1500)
 pg.locator('#ip-msg-text').fill('Myrine, my people crossed the river west of Phaistos. It is fordable at low water.')
 pg.select_option('#ip-msg-tell',label='Alpheios')
 shot(pg,'A3','tell','Idomeneus writes to Myrine: message form with "Tell of a place" set to Alpheios (a river he has seen himself)','#inspect-panel')
 pg.get_by_role('button',name='Send Messenger').click();pg.wait_for_timeout(1500)
 mid=sql(f"SELECT id FROM messengers WHERE story IS NOT NULL ORDER BY sent_at DESC LIMIT 1") if False else sql(f"SELECT id FROM messengers WHERE sender_id='{i['pid']}' ORDER BY sent_at DESC LIMIT 1")
 sql(f"UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='{W}' AND event_type LIKE 'MessengerArrival%' AND payload->>'messenger_id'='{mid}';",True)
 for _ in range(100):
  if sql(f"SELECT status FROM messengers WHERE id='{mid}'")=='delivered':break
  time.sleep(.3)
 pg.context.close()
 for w in (1280,390):
  pg=newpage(br,'Myrine',w);centre(pg,hx['q'],hx['r']);pg.wait_for_timeout(500)
  shot(pg,'A3','told-map',f"Myrine's map around Alpheios ({pl['kind']}), known only from the story")
  open_hex(pg,hx['q'],hx['r']);pg.wait_for_timeout(800)
  shot(pg,'A3','told-panel','Myrine opens one of the told hexes: "Told by Idomeneus"','#inspect-panel')
  pg.context.close()

def fx_ships(hull_dolphin=2):
 """Two galleys in Phaistos's harbour: Dolphin (hull given) and Swallow (hull 5)."""
 W=S['world'];sid=me()['sid'];pid=me()['pid'];q,r=me()['x'],S['r']
 sql(f"DELETE FROM units WHERE owner_id='{pid}' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('{W}','{pid}','galley','naval',0,20,'garrison','{sid}','{sid}','{sid}',{q},{r},'Dolphin',{hull_dolphin}),('{W}','{pid}','galley','naval',0,20,'garrison','{sid}','{sid}','{sid}',{q},{r},'Swallow',5);",True)
 return sql(f"SELECT id FROM units WHERE owner_id='{pid}' AND name='Dolphin'")
@scene('A12','Storm panel and the storm warning before sailing','A storm on the map and its hex panel; then ordering the damaged galley Dolphin (hull 2) to sail: the march preview says that any storm it meets will sink it. Is the warning noticeable, and does it read as a rule (no storm named)?')
def A12(br):
 fx_city();fx_ships();W=S['world'];q,r=me()['x'],S['r']
 dol=sql(f"SELECT id FROM units WHERE owner_id='{me()['pid']}' AND name='Dolphin'")
 sql(f"DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='{W}'); DELETE FROM sea_storms WHERE world_id='{W}';",True)
 st=sql(f"INSERT INTO sea_storms(world_id,heading,created_tick, dies_tick) VALUES('{W}',0,100, 100000) RETURNING id",True).splitlines()[0]
 sql(f"INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('{st}',100,0,{q+2},{r+2}),('{st}',100,1,{q+3},{r+2}),('{st}',100,2,{q+3},{r+3});",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);centre(pg,q+2,r+2);pg.wait_for_timeout(800)
  shot(pg,'A12','map','Map with a storm over three water hexes south-east of Phaistos')
  open_hex(pg,q+3,r+2);pg.wait_for_timeout(800)
  shot(pg,'A12','panel','Hex panel on a storm hex','#inspect-panel')
  pg.evaluate("window.closeInspect&&window.closeInspect()")
  pg.evaluate("async a=>{const m=await import('/static/js/megaron/ui/marchctx.js');m.chooseMarchTarget(a.unit);await m.openMarchCtx({q:a.q,r:a.r,known:true,terrain:'coastal_sea',isSea:true,isSettlement:false,name:'open water'},a.x,85)}",{'unit':dol,'q':q+2,'r':r+2,'x':180 if w<600 else 650})
  pg.wait_for_timeout(2500)
  shot(pg,'A12','warning','March menu for the damaged galley Dolphin (hull 2) toward open water: preview with the storm warning','#mctx, #march-ctx, .march-ctx',pad=14) if pg.locator('#mctx, #march-ctx, .march-ctx').count() else shot(pg,'A12','warning','March menu for Dolphin (hull 2); whole window')
  pg.context.close()

@scene('A10','Codex articles: rites (fixed price list) and lake','Open the Codex on "rites" (the table of fixed divine values: grain 1 ... bronze 33, alphabetical within each worth) and on "lake". Are the table and headings readable, nothing clipped?')
def A10(br):
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w)
  for art,tag,anchor in (('rites','rites','offerings'),('lake','lake',None)):
   pg.evaluate("async id=>{const m=await import('/static/js/megaron/ui/codex.js');await m.openCodex(id)}",art);pg.wait_for_timeout(900)
   shot(pg,'A10',tag+'-top',f'Codex, article "{art}", top of the article','#codex-panel, #drawer-codex, .codex-panel')
   if anchor:
    pg.evaluate("""()=>{const b=document.getElementById('codex-body');const h=[...b.querySelectorAll('h2,h3')].find(x=>/value of an offering/i.test(x.textContent));if(h)h.scrollIntoView({block:'start'})}""");pg.wait_for_timeout(500)
    shot(pg,'A10','rites-table','Codex "rites", section "The value of an offering" with the price table','#codex-panel, #drawer-codex, .codex-panel')
  pg.context.close()
@scene('A15','Forces at the selected hex','Click a hex with forces on it: own and foreign force cards listed under the terrain, collapsed by default (narrow screens), the name opens Forces. Own first. Is it clear which are yours, and is nothing in the foreign card that you should not know?')
def A15(br):
 fx_city();fx_ships();W=S['world'];q,r=me()['x'],S['r'];g=S['actors']['Myrine'];i=me()
 hx=q+3
 sql(f"UPDATE units SET q={hx},r={r},status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='{W}' AND owner_id='{i['pid']}' AND type='spearman' ORDER BY id LIMIT 1); UPDATE units SET q={hx},r={r},status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='{W}' AND owner_id='{g['pid']}' AND type='spearman' ORDER BY id LIMIT 1);",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);centre(pg,hx,r);pg.wait_for_timeout(1000)
  open_hex(pg,hx,r);pg.wait_for_timeout(1200)
  shot(pg,'A15','collapsed','Hex panel with forces, as it opens','#inspect-panel')
  pg.evaluate("document.querySelectorAll('#ip-forces details').forEach(d=>d.open=true)");pg.wait_for_timeout(300)
  shot(pg,'A15','expanded','Same panel with the force cards opened','#inspect-panel')
  pg.context.close()
@scene('A16','Game days in player text','Where durations appear: a build receipt ("N game days - about N hours - ready ..."), the march preview "Arrives in N game days", Recruit costs. Is "game day" used consistently, with wall-clock orientation where there is room?')
def A16(br):
 fx_city();q,r=me()['x'],S['r'];W=S['world'];i=me()
 sql(f"UPDATE units SET status='garrison',settlement_id='{i['sid']}',support_settlement_id='{i['sid']}',q={q},r={r} WHERE world_id='{W}' AND owner_id='{i['pid']}' AND type='spearman'",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  pg.locator('#drawer-city [data-tab="byggnader"]').click();pg.wait_for_timeout(500)
  sel=pg.locator('#city-build-select')
  opts=[o for o in sel.locator('option').all_inner_texts()]
  shot(pg,'A16','build-preview','Buildings tab: construct preview with duration','#city-bld-sec',tall=True) if pg.locator('#city-bld-sec').count() else shot(pg,'A16','build-preview','Buildings tab: construct preview','#drawer-city',tall=True)
  try:
   sel.select_option(index=1);pg.wait_for_timeout(500)
   with pg.expect_response(lambda r_:r_.request.method=='POST' and r_.url.endswith('/build'),timeout=8000):
    pg.locator('#city-bld-sec button[onclick="startBuild()"]').click()
   pg.wait_for_timeout(800)
   shot(pg,'A16','build-receipt','After + Build: the receipt line','#drawer-city',tall=True)
  except Exception as e:PROBLEMS.append(f'A16@{w}: build receipt not captured: {e!r}'[:200])
  pg.evaluate("window.toggleDrawer('city')");pg.wait_for_timeout(300)
  tid=sql(f"SELECT id FROM units WHERE owner_id='{i['pid']}' AND type='spearman' ORDER BY id LIMIT 1")
  pg.evaluate("async a=>{const m=await import('/static/js/megaron/ui/marchctx.js');m.chooseMarchTarget(a.unit);await m.openMarchCtx({q:a.q,r:a.r,known:true,terrain:'plains',isSea:false,isSettlement:false,name:'a plains hex'},a.x,85)}",{'unit':tid,'q':q+3,'r':r-1,'x':180 if w<600 else 650})
  pg.wait_for_timeout(2500)
  shot(pg,'A16','march-preview','March menu preview: arrival in game days','#march-ctx',pad=14)
  pg.context.close()

import html as _html
def build_index(commit,notes=None):
 """Shrink big PNGs, then write index.html and README.md from shots.json + fixture log."""
 from PIL import Image
 shots=json.loads((OUT/'shots.json').read_text());problems=json.loads((OUT/'problems.json').read_text()) if (OUT/'problems.json').exists() else []
 for sh in shots:
  f=OUT/sh['file']
  if f.stat().st_size>400_000:
   im=Image.open(f).convert('RGB').quantize(colors=128,method=Image.Quantize.MEDIANCUT,dither=Image.Dither.NONE);im.save(f,optimize=True)
   sh['quantized']=True
 codes=sorted({x['code'] for x in shots},key=lambda c:int(c[1:]))
 notcap=notes or {}
 e=_html.escape
 out=['<!doctype html><html lang="sv"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bildsvep 2026-11-01</title>',
 '<style>body{font:16px/1.5 Georgia,serif;max-width:1400px;margin:0 auto;padding:1rem 16px;background:#f4ecd8;color:#2a2118}h1,h2{font-family:monospace}h2{border-top:2px solid #c9792a;padding-top:1rem;margin-top:2.5rem}.judge{font-weight:bold}.row{display:flex;flex-wrap:wrap;gap:1rem;align-items:flex-start}figure{margin:0 0 1rem}figure img{max-width:100%;height:auto;border:1px solid #8a7a60;image-rendering:pixelated}figcaption{font-size:.85rem;color:#5a4a38;max-width:640px}code{background:#e8dcc0;padding:0 .2em}.nope li{margin:.3rem 0}</style></head><body>',
 '<h1>Bildsvep 2026-11-01</h1>',
 f'<p>Färska skärmbilder i Firefox (Playwright), egen isolerad rigg, master <code>{e(commit)}</code>, zoom 1,0, bredd 1280 och 390. Alla bilder i originalstorlek (sidan krymper dem bara om fönstret är smalare). Fixturer: <code>README.md</code> i samma mapp.</p>',
 '<p><strong>Svara med koden:</strong> t.ex. “A3 ok, A7 för mörk, A14 behöver egen ton”. Allt som saknar kod nedan är inte fångat — se längst ned.</p>',
 '<ul>'+''.join(f'<li><a href="#{c}">{c}</a> {e(SCENES[c][0])}</li>' for c in codes)+'</ul>']
 for c in codes:
  out.append(f'<h2 id="{c}">{c} · {e(SCENES[c][0])}</h2><p class="judge">Bedöm: <span style="font-weight:normal">{e(SCENES[c][1])}</span></p>')
  for w in (1280,390):
   group=[x for x in shots if x['code']==c and x['width']==w]
   if not group:continue
   out.append(f'<h3>{w} px</h3><div class="row">'+''.join(f'<figure><img src="{e(x["file"])}" alt="{e(c)} {e(x["tag"])}" loading="lazy"><figcaption><b>{e(c)}-{w}-{e(x["tag"])}</b> · {e(x["note"])}</figcaption></figure>' for x in group)+'</div>')
 out.append('<h2 id="saknas">Kunde inte fångas</h2><ul class="nope">'+''.join(f'<li><b>{e(k)}</b>: {e(v)}</li>' for k,v in notcap.items())+'</ul>' if notcap else '')
 if problems:out.append('<h2>Riggens egna anmärkningar</h2><ul>'+''.join(f'<li>{e(x)}</li>' for x in problems)+'</ul>')
 out.append('</body></html>')
 (OUT/'index.html').write_text('\n'.join(out))
 fx=(OUT/'fixture.log.sql').read_text() if (OUT/'fixture.log.sql').exists() else ''
 (OUT/'README.md').write_text(f"""# Bildsvep 2026-11-01

Rigg: `tools/bildsvep_20261101.py` (egna docker-containrar för postgres+redis på slumpad port, server byggd från master `{commit}`, Firefox via Playwright, vy 1280x900 och 390x844, zoom 1,0).
Ingen kod i `server/` eller `web/` är ändrad; live och acceptance-containern är orörda. Världen är 56x40 med en preparerad slätt (rad r-3..r) och kustvatten söder därom; städer: Phaistos (Idomeneus, spelaren), Gournia (Myrine), Pylos (Nestor), Kydonia (Atreus), Knossos (Minos, observatör som ser hela kartan).
Där `tall` användes i skriptet förstorades fönstrets höjd tillfälligt till 1500 px för att få med hela panelen i beskärningen; bredden är alltid 1280/390.

Öppna `index.html`. Fixturloggen nedan är exakt de SQL-satser skriptet körde (samma ordning, per scen). Det som inte står här kom från riktiga API-anrop: registrering, join, grundande av städer, brevskick, byggorder, och UI-klick.

## Fixturer per scen

```sql
{fx}
```
""")
if __name__=='__main__':
 if A.index:
  st=json.load(open(A.keep)) if A.keep and Path(A.keep).exists() else {}
  NC=json.loads((OUT/'notcaptured.json').read_text()) if (OUT/'notcaptured.json').exists() else {}
  build_index(st.get('commit') or run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD'),NC);sys.exit(0)
 try:
  if A.keep and Path(A.keep).exists():S.update(json.load(open(A.keep)))
  else:
   for f in list(OUT.glob('*.png'))+[OUT/n for n in ('shots.json','problems.json','fixture.log.sql','server.log')]:f.unlink(missing_ok=True)
   setup();world_scene()
  if 'Minos' not in S['actors']:
   CUR[0]='world';found('Minos','Knossos',S['q']+28);json.dump(S,open(A.keep,'w'))
  if (OUT/'shots.json').exists():SHOTS.extend(json.loads((OUT/'shots.json').read_text()))
  codes=A.scenes.split(',') if A.scenes else list(SCENES)
  run_scenes(codes)
  (OUT/'problems.json').write_text(json.dumps(PROBLEMS,indent=1))
  print('\n'.join(PROBLEMS) or 'no problems',flush=True)
 finally:
  if A.down or not A.keep:cleanup()
