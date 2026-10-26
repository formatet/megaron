#!/usr/bin/env python3
"""Named march selection proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/marschpanel_bild_20261026.py OUTPUT
Fixtures are logged; no shared-world mutations, mocked responses or DOM replacements.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql','.css','.json') and not p.name.endswith('_test.go')}
before=hashes();fixtures=[];shots=[];observations=[]
(OUT/'source-before.json').write_text(json.dumps(before,indent=2))
def launch(kind,image,args):
 name='megaron-marschpanel-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='marschpanel-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+marschpanel'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'March selection proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   try:return json.load(urllib.request.urlopen(req,timeout=30))
   except urllib.error.HTTPError as e:raise RuntimeError(str(e)+' '+e.read().decode()) from e
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  assert health['commit']==commit and health['migration']==215,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT a.q,a.r FROM map_tiles a WHERE a.world_id='{world}' AND a.q>5 AND a.r>5 AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+20 AND b.r=a.r) AND NOT EXISTS(SELECT 1 FROM provinces p WHERE p.world_id=a.world_id AND p.map_q=a.q AND p.map_r=a.r) ORDER BY a.q,a.r LIMIT 1)t"));q,r=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='{world}'; UPDATE units SET q={q},r={r},status='positioned' WHERE id='{host['id']}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id']
  sql(f"UPDATE settlements SET population=4000 WHERE id='{home}'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{home}'; UPDATE units SET status='disbanded' WHERE world_id='{world}' AND owner_id='{pid}';",True)
  def order_state():return json.loads(sql(f"SELECT coalesce(json_agg(t),'[]') FROM(SELECT id,status,order_payload FROM messengers WHERE world_id='{world}' AND sender_id='{pid}' AND kind='order' ORDER BY id)t"))
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for width,height in [(1280,900),(390,844)]:
    # Three explicit scene cohorts. No game outcome is manufactured; orders below use real browser requests.
    sql(f"UPDATE units SET status='disbanded' WHERE world_id='{world}' AND owner_id='{pid}';",True)
    ids=[]
    for n in range(1,4):
     ids.append(sql(f"INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('{world}','{pid}',NULL,'{home}','spearman','land',100,'positioned',{q+2},{r},{n}) RETURNING id;",True).splitlines()[0])
    units=api(wp+'/units',token=token)['units'];named=[u for u in units if u['id'] in ids];assert len(named)==3,named
    (OUT/f'units-{width}.json').write_text(json.dumps(named,indent=2))
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];posts=[];responses=[]
    page.on('pageerror',lambda e,errors=errors:errors.append(str(e)))
    page.on('request',lambda req,posts=posts:posts.append({'url':req.url,'body':req.post_data}) if req.method=='POST' and ('/march' in req.url or '/recall' in req.url or '/stance' in req.url) else None)
    page.on('response',lambda res,responses=responses:responses.append({'url':res.url,'status':res.status}) if '/march-preview' in res.url else None)
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    def open_panel(x,unknown=False):
     page.evaluate("async a=>{const m=await import('/static/js/megaron/ui/marchctx.js');m.closeMarchCtx();(await import('/static/js/megaron/state.js')).State.marchCtxUnitID=null;await m.openMarchCtx({q:a.q,r:a.r,known:!a.unknown,terrain:'plains',isSea:false,isSettlement:false,name:a.unknown?'Unexplored country':'Plains ('+a.q+','+a.r+')'},a.x,85)}",{'q':x,'r':r,'unknown':unknown,'x':180 if width==390 else 650})
     page.locator('#mg-0-2').wait_for()
    def shot(code):
     page.wait_for_timeout(300);name=f'{code}-{width}.png';page.screenshot(path=str(OUT/name));shots.append({'phase':code,'width':width,'file':name})
    open_panel(q+4)
    assert page.locator('#mctx-units input:checked').count()==1
    text=page.locator('#mctx-units').inner_text();assert all(u['display_name'] in text for u in named),text
    shot('M1-default-one')
    page.locator('#mg-0-0').uncheck();page.locator('#mg-0-2').check();assert page.locator('#mctx-units input:checked').count()==1
    selected=page.evaluate("async()=> (await import('/static/js/megaron/state.js')).State.marchCtxGroups[0].ids[2]")
    selected_name=next(u['display_name'] for u in named if u['id']==selected);assert selected_name=='3rd Spearmen of Lato',selected_name;shot('M2-third-selected')
    before_orders=order_state()
    with page.expect_response(lambda response:response.request.method=='POST' and response.url.endswith('/march')) as result:page.locator('#mctx-send').click()
    assert result.value.status==202,(result.value.status,result.value.text())
    body=result.value.json();after_orders=order_state();new=[o for o in after_orders if o['id'] not in {o['id'] for o in before_orders}]
    assert len(posts)==1 and posts[0]['url'].endswith('/'+selected+'/march'),posts
    assert len(new)==1 and new[0]['order_payload']['unit_id']==selected,(selected,new)
    shot('M3-order-sent')
    (OUT/f'selected-order-{width}.json').write_text(json.dumps({'selected_id':selected,'selected_name':selected_name,'posts':posts,'response':body,'new_messengers':new},indent=2))
    # Long target cannot fit the minimum expedition, even before the Runner can carry an order.
    open_panel(q+20,True);page.locator('#mctx-more').evaluate('(el)=>el.open=true')
    page.locator('#mctx-ticks').fill('6');page.locator('#mctx-ticks').dispatch_event('input')
    count_before=len(posts);orders_before=order_state()
    page.locator('#mctx-send').click();page.wait_for_function("document.getElementById('mctx-err').textContent.includes('Nothing sent')")
    refusal=page.locator('#mctx-err').inner_text();assert len(posts)==count_before and order_state()==orders_before,(posts,refusal)
    assert 'too short' in refusal.lower(),refusal
    shot('M4-short-expedition-refused')
    (OUT/f'short-expedition-{width}.json').write_text(json.dumps({'refusal':refusal,'posts_before':count_before,'posts_after':len(posts),'messengers_unchanged':True,'preview_responses':responses,'errors':errors},indent=2))
    assert not errors,errors
    observations.append({'width':width,'selected':selected,'selected_name':selected_name,'only_one_order':True,'short_expedition_zero_posts':True,'errors':errors})
    ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_sha256':before,'source_unchanged':True,'fixture':'Fresh owned register/join/found Lato through APIs, explicit plains/stock and three positioned named cohort fixture per viewport. Browser uses real application march controller, real units and real network; no DOM or response replacement. Target coordinates are fixture scene inputs. Actual order dispatched by Runner; completion not claimed.','observations':observations,'shots':shots},indent=2))
  (OUT/'index.html').write_text('<!doctype html><meta charset="utf-8"><title>March panel proof</title><style>body{font:16px sans-serif;background:#eee}img{max-width:100%;display:block}figure{margin:2em 0}</style><h1>March panel: named units</h1><p>Real Firefox 1280/390. M1: one default; M2: third chosen; M3: only its Runner dispatched; M4: too short expedition sends nothing.</p>'+''.join('<figure><figcaption>'+s['phase']+' · '+str(s['width'])+'</figcaption><a href="'+s['file']+'"><img src="'+s['file']+'"></a></figure>' for s in shots))
  print('PASS native Firefox1280/390: one default, third named cohort only, actual Runner order, short expedition zero POST/Runner',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
