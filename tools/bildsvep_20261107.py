#!/usr/bin/env python3
"""BILD-svep 2026-11-07: own isolated runtime (docker postgres+redis, built server) and native Firefox.
Usage: python3 tools/bildsvep_20261107.py OUTDIR [--keep STATE.json] [--scenes A1,A2]
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
def fx_city():
 """Idomeneus's Phaistos: plausible stocks (grain/fish/oil with caps), a few buildings, a market board."""
 W=S['world'];sid=me()['sid'];pid=me()['pid']
 sql(f"UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='{sid}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','market',1),('{sid}','barracks',1),('{sid}','stable',1),('{sid}','silo',1),('{sid}','pen',1),('{sid}','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='{me()['province']}'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('{sid}','{W}','{{tin,copper}}','{{pottery,oil}}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '{W}','{pid}',q,r FROM map_tiles WHERE world_id='{W}' AND (abs(q-{me()['x']})+abs(q+r-{me()['x']}-{S['r']})+abs(r-{S['r']}))/2<=5 ON CONFLICT DO NOTHING;",True)
def closeall(pg):
 pg.evaluate("document.querySelectorAll('.drawer.open').forEach(d=>window.toggleDrawer(d.id.replace('drawer-','')))");pg.wait_for_timeout(300)
def hex_screen(pg,q,r):
 return pg.evaluate("""async a=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');
  const p=m.hexPx(a.q,a.r),c=m.canvas.getBoundingClientRect();return {x:c.left+State.camera.x+p.x*m.SCALE*State.camera.zoom,y:c.top+State.camera.y+p.y*m.SCALE*State.camera.zoom}}""",{'q':q,'r':r})
def centre(pg,q,r):
 pg.evaluate("a=>window.centreOn(a.q,a.r)",{'q':q,'r':r});pg.wait_for_timeout(600)


def wp():return '/api/v1/worlds/'+S['world']
def build(who,typ,**kw):
 try:return api(wp()+'/provinces/'+who['province']+'/build','POST',{'building_type':typ,**kw},who['token'])
 except RuntimeError as e:print('build refused',typ,str(e)[-120:])
def place_b(who,typ,good,n):
 for _ in range(n):
  try:api(wp()+'/provinces/'+who['province']+'/placements','POST',{'target_kind':'building','building_type':typ,'good_key':good},who['token'])
  except RuntimeError:pass
def codex(pg,art,tag,code,note):
 pg.evaluate("async id=>{const m=await import('/static/js/megaron/ui/codex.js');await m.openCodex(id)}",art);pg.wait_for_timeout(900)
 shot(pg,code,tag,note,'#codex-panel, #drawer-codex, .codex-panel')

@scene('B1','Builders and Workshop (byggare-verkstad)','Build queue with one active and two waiting builds, the Shipyard and Workshop rows in the city grid (workers placed), the Construct panel, and the Codex article "workshop". Is it clear that more builders = faster work, and which build is running?')
def B1(br):
 fx_city();i=me();sid=i['sid']
 sql(f"DELETE FROM build_queue WHERE settlement_id='{sid}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','shipyard',2),('{sid}','workshop',2) ON CONFLICT DO NOTHING;",True)
 place_b(i,'shipyard','shipbuilding',2);place_b(i,'workshop','building',2)
 build(i,'wall');build(i,'temple',god='potnia');build(i,'foundry')
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  shot(pg,'B1','production','City drawer, Production tab: the city grid with the Shipyard and Workshop rows and their placed workers','#drawer-city',tall=True)
  pg.locator('.gubbe-svg g').filter(has_text='City').first.click();pg.wait_for_timeout(700)
  shot(pg,'B1','grid-city','Production tab after clicking the City hex: the Shipyard and Workshop rows (workers placed, pace) ','#drawer-city',tall=True)
  pg.locator('#drawer-city [data-tab="byggnader"]').click();pg.wait_for_timeout(700)
  shot(pg,'B1','buildings','Buildings tab: Built list, Queue 3/3 note, the build queue (one active, two "next in line") and the Construct panel','#drawer-city',tall=True)
  codex(pg,'workshop','codex','B1','Codex article "workshop"')
  pg.context.close()


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
def places_of(who):
 return api('/api/v1/worlds/'+S['world']+'/places',token=S['actors'][who]['token'])['places']

def prayer_info(pid):
 return next(p for p in api(wp()+'/cult',token=me()['token'])['prayers'] if p['id']==pid)
def unit_target(pid,frag):
 return next(u['id'] for u in prayer_info(pid)['targets']['unit'] if frag in u['name'])
def kult_tab(pg,tab):
 pg.locator(f'#drawer-kult [data-cult-tab="{tab}"]').click();pg.wait_for_timeout(500)

@scene('B6','Prayers 4: Stout Hulls for ever (permanent) and the ship\'s card','Stout Hulls with its fourth step "For ever" (gift x20, one named ship, never a city), then the card of that ship after the prayer: "hull 6/6" and the blessing "Stout Hulls for ever". Does the card make the permanent plank obvious?')
def B6(br):
 fx_prayers();fx_ships(5)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B6','forever','minoan_stillness_hull','Stout Hulls with "For ever" chosen: the ship to choose, gift x20',scope='permanent')
  pg.context.close()
 r=cast('minoan_stillness_hull',target_scope='permanent',target=unit_target('minoan_stillness_hull','Dolphin'))
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'war')
  shot(pg,'B6','card','War drawer, Forces tab: Dolphin after the prayer (hull 6/6, Stout Hulls for ever) beside Swallow (an ordinary hull)','#drawer-war',tall=True)
  pg.context.close()

@scene('B7','Prayers 5a: Fruitful Season, Builder\'s Blessing, the Scapegoat','The Scapegoat chooser (realm only, gift x4); then, after Fruitful Season and the Builder\'s Blessing are laid on Phaistos, the city\'s production panel and the build queue with the blessing marker and a new finish time; Kult -> Kharis with the goat\'s row; and the notice when the gods\' anger fell on the goat instead. Do the blessings show where the player looks for them?')
def B7(br):
 fx_prayers();i=me();sid=i['sid']
 sql(f"DELETE FROM build_queue WHERE settlement_id='{sid}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','workshop',2) ON CONFLICT DO NOTHING;",True)
 place_b(i,'workshop','building',2)
 build(i,'wall');build(i,'olive_press');build(i,'foundry')
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B7','goat-chooser','minoan_potnia_scapegoat','The Scapegoat: one step only (the realm), gift x4 (3 livestock x4 = 12)')
  prayer_offer(pg,'B7','fruitful-chooser','minoan_potnia_fertility','Fruitful Season: this city only / all your cities')
  pg.context.close()
 cast('minoan_potnia_fertility',target_scope='city');cast('minoan_velchanos_building',target_scope='city');cast('minoan_potnia_scapegoat',target_scope='realm')
 sql(f"INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES('{S['world']}','{i['pid']}','DivinePunishmentAverted',3,'{{\"name\":\"Phaistos\",\"would_have\":\"chariot_loss\",\"settlement_id\":\"{sid}\",\"protection_until\":\"2026-11-10T12:00:00+01:00\"}}',100)",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'city')
  shot(pg,'B7','production','City drawer, Production tab after Fruitful Season: the blessing line and growth','#drawer-city',tall=True)
  pg.locator('#drawer-city [data-tab="byggnader"]').click();pg.wait_for_timeout(700)
  shot(pg,'B7','queue','Buildings tab: build queue with the Builder\'s Blessing marker and the new (earlier) finish time','#drawer-city',tall=True)
  pg.evaluate("window.toggleDrawer('city')");pg.wait_for_timeout(300)
  drawer(pg,'kult');kult_tab(pg,'kharis')
  shot(pg,'B7','kharis','Kult -> Kharis with the scapegoat\'s row','#drawer-kult',tall=True)
  pg.evaluate("window.toggleDrawer('kult')");pg.wait_for_timeout(300)
  notifs(pg);shot(pg,'B7','notice','Notifications: the gods\' anger fell on the scapegoat (DivinePunishmentAverted)','#drawer-notif',tall=True,maxh=420)
  pg.context.close()

@scene('B8','Prayers 5b: the Bull\'s Horns and the Thread','The Bull\'s Horns chooser; the card of a unit that has the horns; and the caravan list in Economy where a land caravan under the Thread says so. Is it clear what is blessed and for how long?')
def B8(br):
 fx_prayers();fx_foreign();g=S['actors']['Myrine'];i=me()
 sql(f"UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id='{g['sid']}'",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B8','horns-chooser','minoan_asterion_horns','The Bull\'s Horns: units to choose from, steps unit / city / realm')
  prayer_offer(pg,'B8','thread-chooser','minoan_asterion_thread_v2','The Thread: land caravans, city and realm always open')
  pg.context.close()
 m=deliver_letter(i,g,'Myrine, five timber for fifteen silver, by land.',trade_offer={'kind':'sell','offer_good':'timber','offer_qty':5,'want_silver':15,'by':'land'})
 api(wp()+'/messengers/'+m['id']+'/trade-accept','POST',{},g['token'])
 cast('minoan_asterion_horns',target_scope='unit',target=unit_target('minoan_asterion_horns','1st'))
 cast('minoan_asterion_thread_v2',target_scope='city')
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);drawer(pg,'war')
  shot(pg,'B8','card','War drawer, Forces tab: the unit with the Bull\'s Horns','#drawer-war',tall=True)
  pg.evaluate("window.toggleDrawer('war')");pg.wait_for_timeout(300)
  drawer(pg,'economy');pg.locator('#drawer-economy [data-tab="automation"]').click();pg.wait_for_timeout(1500)
  shot(pg,'B8','caravan','Economy -> Routes & cargo, "Other cargo in transit": the caravan to Gournia is under the Thread (12 game days) - see the report: this list does not show it','#drawer-economy',tall=True,maxh=700)
  pg.context.close()

@scene('B9','Prayers 5c: The Walls Fall (city chooser) and the target\'s two notices','Kult -> Prayers: The Walls Fall names a foreign city whose wall you remember. Then the other side: Nestor, whose city it was, gets WallsOmen at the cast (he stands well with the gods) and WallsFell three game days later; never who prayed. Do the two notices read right, and do they tell him nothing he should not know?')
def B9(br):
 fx_prayers();fx_foreign();py=S['actors']['Nestor'];i=me();W=S['world']
 sql(f"UPDATE settlements SET wall_level=2 WHERE id='{py['sid']}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{py['sid']}','wall',2) ON CONFLICT DO NOTHING; UPDATE buildings SET level=2 WHERE settlement_id='{py['sid']}' AND building_type='wall'; UPDATE player_seen_settlements SET wall_level=2 WHERE player_id='{i['pid']}' AND settlement_id='{py['sid']}'; UPDATE player_world_records SET kharis_amount=80,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='{W}' AND player_id='{py['pid']}';",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B9','chooser','minoan_potnia_walls_fall','The Walls Fall: the city chooser lists Pylos (held by Nestor, wall level as last seen)')
  pg.context.close()
 r=cast('minoan_potnia_walls_fall',target=py['sid'])
 if not r.get('success',True) or r.get('status'):PROBLEMS.append(f'B9: walls fall cast: {r}')
 sql(f"UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='{W}' AND event_type ILIKE '%wallsfall%'",True)
 for _ in range(60):
  if sql(f"SELECT count(*) FROM notifications WHERE player_id='{py['pid']}' AND kind='WallsFell'")!='0':break
  time.sleep(.5)
 for w in (1280,390):
  pg=newpage(br,'Nestor',w);notifs(pg)
  shot(pg,'B9','owner-notices','Nestor\'s notifications: the omen at the cast and the fall (the wall lost a level), no name of the one who prayed','#drawer-notif',tall=True,maxh=420)
  pg.context.close()

@scene('B10','Prayers 5d: the Bird Oracle','The Bird Oracle chooser (Cities / Named places); the map under the oracle day, where the area around Pylos is live with foreign forces in sight; and the day after, when it is grey memory. Is the difference between live and memory unmistakable?')
def B10(br):
 fx_prayers();fx_foreign();py=S['actors']['Nestor'];i=me();W=S['world'];r0=S['r']
 mine=[p for p in places_of('Idomeneus') if p['kind'] in('river','plain','hills','cape','island','delta','lake','strait') and p['hexes']][0];a,b=mine['id'].split(',')
 sql(f"INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick,name,named_tick) VALUES('{W}','{mine['kind']}',{a},{b},'{i['pid']}',90,'Alpheios',90) ON CONFLICT (world_id,kind,anchor_q,anchor_r) DO UPDATE SET name='Alpheios',claimed_by='{i['pid']}'; UPDATE units SET q={py['x']+1},r={r0},status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='{W}' AND owner_id='{py['pid']}' AND type='spearman' LIMIT 1);",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B10','chooser','minoan_asterion_bird_oracle','The Bird Oracle: optgroups Cities (Phaistos, remembered Pylos, Kydonia) and Named places (Alpheios)')
  pg.context.close()
 r=cast('minoan_asterion_bird_oracle',target=py['sid'])
 if r.get('status') or not r.get('success',True):PROBLEMS.append(f'B10: oracle cast {r}')
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);centre(pg,py['x']+1,r0);pg.evaluate('window.dismissAllChips&&window.dismissAllChips()');pg.wait_for_timeout(1500)
  shot(pg,'B10','during','Map during the oracle day, centred on Pylos: live sight over the place, Nestor\'s unit next to the city in sight')
  pg.context.close()
 sql(f"UPDATE oracle_sights SET until_milli=from_milli+1 WHERE player_id='{i['pid']}'",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);centre(pg,py['x']+1,r0);pg.evaluate('window.dismissAllChips&&window.dismissAllChips()');pg.wait_for_timeout(1500)
  shot(pg,'B10','after','The day after (the sight has ended): the same place as grey memory, no live units')
  pg.context.close()

def announce_release(commit):
 tmp=Path(S['tmp']);run('go','build','-o',str(tmp/'announce'),'./cmd/announce-keryx-release',cwd=ROOT/'server',env=clean)
 port=json.loads(run('docker','inspect',S['pgc']))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 return run(str(tmp/'announce'),commit,env={**clean,'DATABASE_URL':f'postgres://postgres:bildsvep@localhost:{port}/bildsvep?sslmode=disable'})
@scene('B11','keryx-release: the notice "A new keryx is out"','The KeryxReleased notice in Notifications, written by the real announce tool: text with the commit and a Download link. Is it noticeable as news and not as an alarm, and does the link read as a link?')
def B11(br):
 FIX.append('-- scene B11\n-- notice written by the real tool: announce-keryx-release 1a2b3c4d5e')
 out=announce_release('1a2b3c4d5e'+secrets.token_hex(1));print(out)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);notifs(pg)
  shot(pg,'B11','notice','Notifications: KeryxReleased',"#drawer-notif",tall=True,maxh=420)
  pg.context.close()


@scene('B3','Prayers 2b: the target chooser of Swift Feet and Following Seas','Swift Feet (Runners and land caravans) and Following Seas (ships): the offering row with the steps. For Swift Feet the city and realm steps are always open (they hold for departures too), the unit step lists Runners/caravans on the road. Is it clear what is blessed and for how long?')
def B3(br):
 fx_prayers()
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B3','swift-feet','minoan_asterion_swift','Swift Feet: steps one Runner or caravan / this city\'s / all of yours, always open for city and realm')
  prayer_offer(pg,'B3','following-seas','minoan_stillness_swift','Following Seas: ships to choose from, steps unit / city / fleet')
  pg.context.close()

@scene('B4','Storms 3a: shrunk storms and a dead storm\'s memory','The map with named storms: one of 2 hexes and one of 1 hex (live, in sight), and the grey memory cloud of a storm that died while out of sight. Do the small storms still read as storms, and is the remembered one clearly grey/old?')
def B4(br):
 fx_prayers();q,r=me()['x'],S['r']
 clear_storms();mk_storm('Notos',[(q+1,r+2),(q+2,r+2)]);mk_storm('Kaikias',[(q-2,r+2)])
 mk_storm('Zephyros',[(q+8,r+2),(q+9,r+2),(q+8,r+3)],dead=True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);centre(pg,q+1,r+2);pg.wait_for_timeout(1000)
  shot(pg,'B4','live','Two live storms south of Phaistos: Notos (2 hexes, right) and Kaikias (1 hex, left)')
  open_hex(pg,q+2,r+2);pg.wait_for_timeout(600)
  shot(pg,'B4','panel','Hex panel on one hex of Notos','#inspect-panel')
  pg.evaluate("window.closeInspect&&window.closeInspect()")
  centre(pg,q+6,r+2);pg.wait_for_timeout(1000)
  shot(pg,'B4','memory','Zephyros, a storm that died while out of sight: its memory cloud (grey, dithered)')
  pg.context.close()

@scene('B5','Storms 3b: Still the Storm (storm chooser) and Wake the Storm (hex field)','Kult -> Prayers: Still the Storm lists the storms you have named and keep on your storm map; Wake the Storm asks for a sea hex as q,r. Is the instruction text clear, is the field usable?')
def B5(br):
 fx_prayers();q,r=me()['x'],S['r']
 clear_storms();mk_storm('Notos',[(q+1,r+2),(q+2,r+2)]);mk_storm('Kaikias',[(q-2,r+2)])
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B5','still','minoan_stillness_calm','Still the Storm: choose one of your named storms (with where it was last seen)')
  row=pg.locator('#drawer-kult [data-cult-prayer$="__minoan_stillness_wake"]')
  row.locator('button:has-text("Offer"), button:has-text("Compose offering")').first.click();pg.wait_for_timeout(900)
  row.locator('.offer-target-hex').fill(f'{q+6},{r+2}');row.scroll_into_view_if_needed()
  shot(pg,'B5','wake','Wake the Storm: the hex field filled in as q,r',f'#drawer-kult [data-cult-prayer$="__minoan_stillness_wake"]',pad=6,tall=True)
  pg.context.close()


def fx_ships(hull_dolphin=2):
 """Two galleys in Phaistos's harbour: Dolphin (hull given) and Swallow (hull 5)."""
 W=S['world'];sid=me()['sid'];pid=me()['pid'];q,r=me()['x'],S['r']
 sql(f"DELETE FROM units WHERE owner_id='{pid}' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('{W}','{pid}','galley','naval',0,20,'garrison','{sid}','{sid}','{sid}',{q},{r},'Dolphin',{hull_dolphin}),('{W}','{pid}','galley','naval',0,20,'garrison','{sid}','{sid}','{sid}',{q},{r},'Swallow',5);",True)
def fx_prayers():
 """Four temples in Phaistos (Potnia, Velchanos, Asterion, Sea Daimons), kharis 90, two galleys, plenty of goods."""
 fx_city();fx_ships();clear_storms();W=S['world'];sid=me()['sid'];pid=me()['pid']
 sql(f"DELETE FROM buildings WHERE settlement_id='{sid}' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{sid}','temple',3,'potnia'),('{sid}','temple',3,'velchanos'),('{sid}','temple',3,'asterion'),('{sid}','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='{W}' AND player_id='{pid}'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='{sid}'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;",True)
def cast(prayer,**kw):
 r=api(wp()+'/settlements/'+me()['sid']+'/rite','POST',{'prayer':prayer,**kw},me()['token'],raw=True)
 sql("DELETE FROM temple_rest")
 if r.get('status') if 'status' in r else False:PROBLEMS.append(f'{CUR[0]}: cast {prayer} refused {r}')
 return r
def clear_storms():
 sql(f"DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='{S['world']}'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='{S['world']}'); DELETE FROM sea_storms WHERE world_id='{S['world']}';",True)
def mk_storm(name,hexes,dead=False,seen=True):
 """A named storm of Idomeneus; seen=True gives him a sighting (memory row)."""
 W=S['world'];pid=me()['pid']
 sid=sql(f"INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick{',died_tick' if dead else ''}) VALUES('{W}',2,60,{95 if dead else 100000},'{name}','{pid}',70,70{',95' if dead else ''}) RETURNING id",True).splitlines()[0]
 sql("INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES "+','.join(f"('{sid}',100,{k},{q},{r})" for k,(q,r) in enumerate(hexes)),True)
 if seen:sql(f"INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('{pid}','{sid}',{90 if dead else 100},'{json.dumps([{'q':q,'r':r} for q,r in hexes])}')",True)
 return sid
def notifs(pg):
 pg.locator('#gt-notif-btn').click();pg.wait_for_timeout(1000)
def open_prayers(pg):
 drawer(pg,'kult');pg.locator('#drawer-kult [data-cult-tab="prayers"]').click();pg.wait_for_timeout(500)
def prayer_offer(pg,code,tag,pid,note,scope=None,full=False):
 """Open the offering row of one prayer (click Offer) and shoot that prayer's block."""
 row=pg.locator(f'#drawer-kult [data-cult-prayer$="__{pid}"]')
 row.locator('button:has-text("Offer"), button:has-text("Compose offering")').first.click();pg.wait_for_timeout(900)
 if scope:row.locator(f'input[type=radio][value="{scope}"]').check();pg.wait_for_timeout(300)
 row.scroll_into_view_if_needed();pg.wait_for_timeout(300)
 shot(pg,code,tag,note,f'#drawer-kult [data-cult-prayer$="__{pid}"]',pad=6,tall=True)
 row.locator('button:has-text("Cancel")').first.click();pg.wait_for_timeout(300)

@scene('B2','Prayers 2a: the target chooser (The Young God\'s Arm, Stout Hulls) and the warded-storm notice','Kult -> Prayers: the offering row of The Young God\'s Arm with its three steps (one named unit / this city\'s / whole realm) and the gift multiplier per step, then the same for Stout Hulls (ships). Last: the storm notice when a Sea Daimons blessing took the blow. Is it clear who is blessed and that wider = dearer?')
def B2(br):
 fx_prayers()
 sql(f"INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES('{S['world']}','{me()['pid']}','ShipStormDamaged',2,'{{\"name\":\"Dolphin\",\"ship_type\":\"galley\",\"q\":{me()['x']+2},\"r\":{S['r']+2},\"hull\":5,\"hull_max\":5,\"divine\":\"warded\",\"storm_name\":\"Notos\"}}',100)",True)
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);open_prayers(pg)
  prayer_offer(pg,'B2','arm-unit','minoan_velchanos_strength','The Young God\'s Arm, step "One named" (default): a unit to choose from, gift x1')
  prayer_offer(pg,'B2','arm-realm','minoan_velchanos_strength','The Young God\'s Arm with "The whole realm" chosen: the fields follow the step (gift x4)',scope='realm')
  prayer_offer(pg,'B2','hulls','minoan_stillness_hull','Stout Hulls: ships to choose from, steps unit / city / fleet')
  pg.context.close()
 for w in (1280,390):
  pg=newpage(br,'Idomeneus',w);notifs(pg)
  shot(pg,'B2','notice','Notifications: the Sea Daimons\' blessing took the blow (storm notice, ship warded)','#drawer-notif',tall=True,maxh=420)
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
 out=['<!doctype html><html lang="sv"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bildsvep 2026-11-07</title>',
 '<style>body{font:16px/1.5 Georgia,serif;max-width:1400px;margin:0 auto;padding:1rem 16px;background:#f4ecd8;color:#2a2118}h1,h2{font-family:monospace}h2{border-top:2px solid #c9792a;padding-top:1rem;margin-top:2.5rem}.judge{font-weight:bold}.row{display:flex;flex-wrap:wrap;gap:1rem;align-items:flex-start}figure{margin:0 0 1rem}figure img{max-width:100%;height:auto;border:1px solid #8a7a60;image-rendering:pixelated}figcaption{font-size:.85rem;color:#5a4a38;max-width:640px}code{background:#e8dcc0;padding:0 .2em}.nope li{margin:.3rem 0}</style></head><body>',
 '<h1>Bildsvep 2026-11-07</h1>',
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
 (OUT/'README.md').write_text(f"""# Bildsvep 2026-11-07

Rigg: `tools/bildsvep_20261107.py` (egna docker-containrar för postgres+redis på slumpad port, server byggd från master `{commit}`, Firefox via Playwright, vy 1280x900 och 390x844, zoom 1,0).
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
