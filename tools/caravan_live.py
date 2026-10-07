#!/usr/bin/env python3
"""Real caravan player lifecycle in own fresh PostgreSQL16/Redis7.
Usage: caravan_live.py OUT BUILD_COMMIT MIGRATION
OUT must contain temenos and keryx. No SQL writes, inherited .env or production access.
"""
import json, os, secrets, socket, subprocess, sys, time, urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
ROOT=Path(sys.argv[4]).resolve() if len(sys.argv)>4 else Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve(); EXPECTED=sys.argv[2]; MIGRATION=int(sys.argv[3])
PREFIX='megaron-caravan-'+secrets.token_hex(4); containers=[]; proc=None; configs=[]
OUT.mkdir(parents=True,exist_ok=True)
def command(*args): return subprocess.check_output(args,text=True).strip()
def launch(service,image,args):
    name=PREFIX+'-'+service; port='5432' if service=='pg' else '6379'
    command('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image); containers.append(name)
    ports=json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports']
    return int(ports[port+'/tcp'][0]['HostPort'])
def wait(label,fn,timeout=200):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        result=fn()
        if result: return result
        time.sleep(.35)
    raise AssertionError('timeout: '+label)
try:
    pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=caravan-pw','-e','POSTGRES_DB=caravan'])
    redis=launch('redis','redis:7-alpine',[])
    wait('postgres',lambda: subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0,30)
    time.sleep(1) # postgres image may restart after its initial initdb ready signal
    with socket.socket() as sock: sock.bind(('127.0.0.1',0)); port=sock.getsockname()[1]
    base=f'http://127.0.0.1:{port}'
    env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'DATABASE_URL':f'postgres://postgres:caravan-pw@127.0.0.1:{pg}/caravan?sslmode=disable','REDIS_URL':f'127.0.0.1:{redis}','JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Caravan proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(OUT/'chronicles'),'REPORTS_DIR':str(OUT/'reports')}
    assert not (ROOT/'server/.env').exists()
    def api(path,method='GET',data=None,token=None):
        req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
        try:
            with urllib.request.urlopen(req,timeout=20) as response:return json.load(response)
        except Exception as e:
            if hasattr(e,'read'): raise RuntimeError(path+': '+e.read().decode()) from None
            raise
    def sql(query):
        assert query.strip().upper().startswith('SELECT '), 'ground inspection is read-only'
        return command('docker','exec',containers[0],'psql','-U','postgres','-d','caravan','-tAc',query)
    log=(OUT/'server.log').open('w'); proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
    def healthy():
        if proc.poll() is not None:raise RuntimeError('server exited')
        try:return api('/healthz')
        except Exception:return None
    health=wait('health',healthy,90); assert health['commit']==EXPECTED and health['migration']==MIGRATION,health
    worlds=api('/api/v1/worlds'); world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id']; wp='/api/v1/worlds/'+world
    players=[]
    for n in range(16):
        token=api('/api/v1/auth/register','POST',{'username':'caravan'+secrets.token_hex(5),'password':secrets.token_urlsafe(32)})['access_token']
        api(wp+'/join','POST',{},token)
        status=api(wp+'/founding/status',token=token)
        cfg=OUT/f'private-config-{n}.json'; cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world})); cfg.chmod(0o600); configs.append(cfg)
        players.append({'token':token,'cfg':cfg,'status':status})
    def cli(p,*args):
        return subprocess.check_output([str(OUT/'keryx'),*args],env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(p['cfg'])},text=True,stderr=subprocess.PIPE)
    # Choose a reproducible viable scenario only from player-visible map data.
    # Hosts may walk on unknown terrain; the server owns and validates A*.
    (OUT/'spawn-public.json').write_text(json.dumps([p['status'] for p in players],indent=2)+'\n')
    def dist(a,b):return max(abs(a['q']-b['q']),abs(a['r']-b['r']),abs(a['q']+a['r']-b['q']-b['r']))
    pairs=sorted(((dist(a['status'],b['status']),i,j) for i,a in enumerate(players) for j,b in enumerate(players) if i<j))
    selected=None
    for _,i,j in pairs:
        a,b=players[i],players[j]
        orders=[]; visited=set()
        for step in range(10):
            current=api(wp+'/founding/status',token=b['token'])
            if 5<=dist(current,a['status'])<=6:
                selected=(a,b,current,orders); break
            tiles=api(wp+'/map',token=b['token']); tiles=tiles if isinstance(tiles,list) else tiles['tiles']
            candidates=[t for t in tiles if t.get('visible') and t.get('terrain') not in ('sea','mountain') and dist(t,a['status'])>=5 and (t['q'],t['r']) not in visited and dist(t,current)>0 and dist(t,a['status'])<dist(current,a['status'])]
            candidates.sort(key=lambda t:(dist(t,a['status']),-dist(t,current)))
            moved=False
            for target in candidates:
                try:
                    result=json.loads(cli(b,'march','--unit',current['host_unit_id'],'--target',f"{target['q']},{target['r']}",'--json'))
                    orders.append(result); visited.add((target['q'],target['r'])); moved=True
                    wait('host arrival',lambda: next(u for u in api(wp+'/units',token=b['token'])['units'] if u['id']==current['host_unit_id'])['status']!='marching')
                    break
                except subprocess.CalledProcessError:continue
            if not moved:break
        if selected:break
    assert selected,'no player-visible land founding site with reachable host'
    a,b,target,march=selected
    print('hosts travelled',flush=True)
    for p,name in ((a,'CaravanAlpha'),(b,'CaravanBeta')):
        founded=api(wp+'/founding/settle','POST',{'name':name},p['token']); p['founded']=founded; p['name']=name
        cfg=json.loads(p['cfg'].read_text()); cfg['province_id']=founded['province_id']; p['cfg'].write_text(json.dumps(cfg))
    # A real expedition discovers the neighbouring city; its messenger gives B
    # physical contact with the origin. B's original escort need not teleport
    # with the wandering host when it founds a city.
    data=api(wp+'/units',token=a['token']); scout=next(u for u in data['units'] if u.get('deployable') and u['status']=='garrison' and u['category']=='land')
    result=json.loads(cli(a,'march','--unit',scout['id'],'--target',f"{target['q']},{target['r']}",'--intent','explore','--ticks',str(data['expedition_rules']['max_ticks']),'--json'))
    wait('scout contact',lambda:any(x.get('settlement_id')==b['founded']['settlement_id'] for x in api(wp+'/provinces',token=a['token'])))
    contact_message=json.loads(cli(a,'message','--to',b['name'],'--text','A physical greeting before our trade','--json'))
    contact_mid=contact_message.get('messenger_id',contact_message.get('id'))
    wait('contact messenger',lambda:any(m['id']==contact_mid for m in api(wp+'/messengers/inbox',token=b['token'])))
    contacts=[{'scout_id':scout['id'],'order':result,'messenger':contact_message}]
    print('contacts established',flush=True)
    proof={'health':health,'registered_joined':len(players),'founded':[p['founded'] for p in (a,b)],'host_order':march,'contacts':contacts,'trades':[],'browser_errors':[],'scope':'Fresh own PostgreSQL16/Redis7, clean server environment, real player register/join/march/found, actual compiled CLI and browser buttons; SQL SELECT only.'}
    def transports():
        fields="t.id,kind,status,category,due_tick,origin_q,origin_r,dest_q,dest_r,ship_unit_id,(SELECT json_object_agg(g.good_key,g.quantity) FROM transport_goods g WHERE g.transport_id=t.id) manifest"
        if MIGRATION>=160:fields+=',journey,departed_tick'
        return json.loads(sql(f"SELECT coalesce(json_agg(x),'[]') FROM (SELECT {fields} FROM transports t ORDER BY departs_at,t.id) x"))
    with sync_playwright() as pw:
        browser=pw.chromium.launch()
        for kind in ('buy','sell'):
            args=('trade-offer','--to',b['name'],'--want-good','grain','--want-qty','1','--offer-silver','1','--json') if kind=='buy' else ('trade-offer','--to',b['name'],'--offer-good','grain','--offer-qty','1','--want-silver','1','--json')
            offered=json.loads(cli(a,*args)); mid=offered.get('messenger_id',offered.get('id')); assert mid,offered
            wait('offer delivery',lambda:any(m['id']==mid for m in api(wp+'/messengers/inbox',token=b['token'])))
            before=transports()
            page=browser.new_page(viewport={'width':1280,'height':900}); page.on('pageerror',lambda e:proof['browser_errors'].append(str(e)))
            page.goto(base+'/'); page.evaluate('t=>localStorage.setItem("poleia_token",t)',b['token']); page.context.add_cookies([{'name':'poleia_token','value':b['token'],'url':base}]); page.goto(base+'/play',wait_until='networkidle')
            page.wait_for_function('window.openDrawer !== undefined')
            page.evaluate('''async()=>{document.getElementById('drawer-diplomacy').classList.add('open');const d=await import('/static/js/megaron/ui/drawers/diplomacy.js');await d.loadDiplomacyDrawer();}''')
            block=page.locator('#dip-trade-'+mid); expect(block).to_be_attached()
            page.locator('.dip-thread').evaluate_all('(xs)=>xs.forEach(x=>x.setAttribute("data-open",""))')
            with page.expect_response(lambda r: '/messengers/'+mid+'/trade-accept' in r.url and r.request.method=='POST') as response: block.get_by_role('button',name='Accept ✓',exact=True).click()
            accepted=response.value.json(); assert response.value.status==200,accepted
            expect(block).to_contain_text('Accepted'); page.locator('#drawer-diplomacy').screenshot(path=str(OUT/f'{kind}-accept.png'))
            page.close()
            dispatch=transports(); new=[t for t in dispatch if t['id'] not in {x['id'] for x in before}]; assert new,new
            if MIGRATION>=160:
                for t in new:
                    j=t['journey']; assert j and t['due_tick']-t['departed_tick']==j['travel_ticks'],t
                    assert j['travel_ticks']==max(1,(sum(j['step_costs'])*3+1000)//2000),t
                    assert len(j['step_costs'])==len(j['path'])-1==j['distance'],t
            def completed():
                ts=transports()
                return ts if len(ts)>len(before) and all(t['status']!='in_transit' for t in ts) else None
            done=wait('trade caravan lifecycle',completed,240)
            proof['trades'].append({'kind':kind,'offered':offered,'accepted':accepted,'dispatch':new,'completed':done,'cli_cargo':cli(a,'cargo'),'cli_outbox':cli(a,'outbox')})
            print(kind+' trade completed',flush=True)
        browser.close()
    assert not proof['browser_errors'],proof['browser_errors']
    proof['ground']=json.loads(sql("SELECT json_build_object('migration',(SELECT version FROM schema_migrations),'dirty',(SELECT dirty FROM schema_migrations),'failed_jobs',(SELECT count(*) FROM scheduled_events WHERE failed_at IS NOT NULL),'transports',(SELECT count(*) FROM transports),'in_transit',(SELECT count(*) FROM transports WHERE status='in_transit'))"))
    assert not proof['ground']['dirty'] and proof['ground']['failed_jobs']==proof['ground']['in_transit']==0,proof['ground']
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n'); print('PASS caravan player lifecycle',flush=True)
finally:
    if proc is not None:proc.terminate(); proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-f',name],capture_output=True)
    for cfg in configs:cfg.unlink(missing_ok=True)
