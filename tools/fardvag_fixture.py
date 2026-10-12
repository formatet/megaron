#!/usr/bin/env python3
"""Explicit isolated route BILD fixture; frozen world, normal server APIs/FOW.
No claim of natural gameplay. Does not alter the existing acceptance world.
"""
import argparse,datetime,json,pathlib,subprocess,urllib.request,uuid
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/fardvag';BASE='http://localhost:8097'
PW='acceptance-pw-123'
def api(path,token=None,data=None):
    req=urllib.request.Request(BASE+'/api/v1/'+path,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})},data=None if data is None else json.dumps(data).encode())
    return json.load(urllib.request.urlopen(req,timeout=30))
def sql(query):return subprocess.check_output(['tools/acceptance.sh','psql',query],cwd=ROOT,text=True).strip()
def literal(x):return "'"+str(x).replace("'","''")+"'"
def new_id(query):return sql(query+' RETURNING id').splitlines()[0]
def utc(t):return t.isoformat().replace('+00:00','Z')
def credentials():
    token=api('auth/login',data={'username_or_email':'Eirene','password':PW})['access_token']
    return token,api('auth/me',token)['id']
def create():
    token,viewer=credentials();now=datetime.datetime.now(datetime.timezone.utc);wid=str(uuid.uuid4())
    foreign_name='FardvagPriam'
    try:foreign_token=api('auth/login',data={'username_or_email':foreign_name,'password':PW})['access_token']
    except urllib.error.HTTPError:foreign_token=api('auth/register',data={'username':foreign_name,'password':PW,'email':foreign_name+'@example.test'})['access_token']
    foreign=api('auth/me',foreign_token)['id']
    sql(f"UPDATE players SET wanax_name='Priam of Tyre' WHERE id='{foreign}'")
    sql(f"INSERT INTO worlds(id,name,status,state,current_tick,last_tick_at,map_width,map_height) VALUES('{wid}','Fardvag {wid[:8]}','archived','active',128,{literal(utc(now-datetime.timedelta(seconds=20)))},21,15)")
    sql(f"INSERT INTO map_tiles(world_id,q,r,terrain,fertility,mineral) SELECT '{wid}',q,r,CASE WHEN q>=11 THEN 'coastal_sea' WHEN q BETWEEN 3 AND 5 AND r BETWEEN 2 AND 4 THEN 'forest_cedar' WHEN q BETWEEN 7 AND 10 AND r BETWEEN 7 AND 11 THEN 'semi_desert' ELSE 'plains' END,.5,.5 FROM generate_series(0,20) q CROSS JOIN generate_series(0,14) r")
    def city(name,owner,q,r):
        pid=new_id(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES('{wid}',{q},{r},'plains')")
        return new_id(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES('{wid}','{pid}',{literal(name)},'achaean','{owner}','capital',true,1000)")
    home=city('Mycenae',viewer,5,7);other=city('Tyre',foreign,9,4);town=city('Knossos',viewer,7,11)
    def unit(name,owner,kind,category,path):
        costs=[1000]*(len(path)-1);route={'start_tick':120,'end_tick':140,'hexes':path,'costs':costs}
        return new_id(f"INSERT INTO units(world_id,owner_id,settlement_id,type,category,size,crew,hull,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route,departs_at,arrives_at,name) VALUES('{wid}','{owner}','{home if owner==viewer else other}','{kind}','{category}',100,30,2,'marching',{path[0][0]},{path[0][1]},{path[-1][0]},{path[-1][1]},120,140,{literal(json.dumps(route))},{literal(utc(now-datetime.timedelta(seconds=48)))},{literal(utc(now+datetime.timedelta(seconds=72)))},{literal(name)})")
    actors=[]
    def record(label,kind,id,path,own):actors.append({'label':label,'kind':kind,'id':id,'path':path,'own':own})
    army=[[q,4] for q in range(2,9)];ship=[[13,r] for r in range(1,8)]
    record('Spearmen of Lykoros','army',unit('Eumedes',viewer,'spearman','land',army),army,True)
    record('Doria','ship',unit('Doria',viewer,'galley','naval',ship),ship,True)
    enemy_army=[[8,r] for r in range(2,7)];enemy_ship=[[15,r] for r in range(1,7)]
    record('Spearmen of Priam of Tyre','foreign-army',unit('Spearmen of Priam',foreign,'spearman','land',enemy_army),enemy_army,False)
    record('Galley of Tyre','foreign-ship',unit('Galley of Tyre',foreign,'galley','naval',enemy_ship),enemy_ship,False)
    # Own cities and active units supply normal LIVE eyes, with no tier overrides.
    def caravan(owner,kind,from_city,to_city,path):
        duration=((len(path)-1)*3000+1000)//2000
        journey={'category':'land','path':[{'q':q,'r':r} for q,r in path],'step_costs':[1000]*(len(path)-1),'travel_ticks':duration,'distance':len(path)-1}
        tid=new_id(f"INSERT INTO transports(world_id,owner_id,kind,category,origin_id,dest_id,origin_q,origin_r,dest_q,dest_r,journey,departed_tick,due_tick,departs_at,arrives_at) VALUES('{wid}','{owner}','{kind}','land','{from_city}','{to_city}',{path[0][0]},{path[0][1]},{path[-1][0]},{path[-1][1]},{literal(json.dumps(journey))},125,{125+duration},{literal(utc(now-datetime.timedelta(seconds=18)))},{literal(utc(now+datetime.timedelta(seconds=(duration-3)*6)))})")
        sql(f"INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES('{tid}','grain',25)")
        return tid
    cpath=[[4,r] for r in range(7,13)];record('Caravan of Lykoros','caravan',caravan(viewer,'transfer',home,town,cpath),cpath,True)
    agreement=[[6,r] for r in range(1,7)];record('Agreed caravan of Priam','agreed',caravan(foreign,'trade',other,home,agreement),agreement,False)
    gift=[[9,r] for r in range(5,12)];record('Gift from Priam','gift',caravan(foreign,'gift',other,town,gift),gift,False)
    def runner(owner,from_city,to_city,path):
        start=now-datetime.timedelta(hours=1);end=now+datetime.timedelta(hours=1)
        return new_id(f"INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,kind,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES('{wid}','{owner}','{from_city}','{to_city}','message','BILD route proof','outbound',{path[0][0]},{path[0][1]},{literal(utc(start))},{literal(utc(end))})")
    rpath=[[5,7],[6,7],[6,8],[7,8],[7,9],[7,10],[7,11]]
    record('Runner of Lykoros','runner',runner(viewer,home,town,rpath),rpath,True)
    incoming=[[9,4],[8,4],[7,5],[6,6],[5,7]]
    record('Runner of Priam','incoming-runner',runner(foreign,other,home,incoming),incoming,False)
    scenario={'world':wid,'viewer':viewer,'foreign':foreign,'actors':actors,'created_at':utc(now),'fixture':'explicit SQL frozen-world movement snapshot plus normal legal live eyes; no browser payload or FOW overrides; not natural dispatch/playthrough'}
    (OUT/'scenario.json').write_text(json.dumps(scenario,indent=2)+'\n')
    return scenario

def capture(label):
    token,_=credentials();s=json.loads((OUT/'scenario.json').read_text());folder=OUT/label;folder.mkdir(exist_ok=True)
    health=json.load(urllib.request.urlopen(BASE+'/healthz'));result={'health':health,'world':s['world'],'source_base':'746398ed' if label=='baseline' else subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()}
    for name in ['','map','units','foreign-units','messengers','trades']:
        data=api(f'worlds/{s["world"]}'+('/'+name if name else ''),token);result[name or 'world_info']=data
    (folder/'payloads.json').write_text(json.dumps(result,indent=2)+'\n')
    print(label,health,s['world'])

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('mode',choices=['create','baseline','after']);args=parser.parse_args()
    OUT.mkdir(parents=True,exist_ok=True)
    if args.mode=='create':create()
    else:capture(args.mode)
