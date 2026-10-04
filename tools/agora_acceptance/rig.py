#!/usr/bin/env python3
"""Isolated Continuwuity fixtures. Never writes credentials to stdout."""
import argparse,json,os,secrets,time,urllib.request,urllib.error,urllib.parse,uuid
from pathlib import Path

def private_json(path,data):
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
    with os.fdopen(fd,'w') as f:json.dump(data,f)

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action',choices=['init','bootstrap','verify'])
    parser.add_argument('--dir',required=True,type=Path)
    parser.add_argument('--port',type=int,default=18099)
    args=parser.parse_args();root=args.dir
    base=f'http://127.0.0.1:{args.port}'
    if args.action=='init':
        root.mkdir(mode=0o700,parents=True,exist_ok=True);os.chmod(root,0o700)
        if (root/'secret.json').exists():raise SystemExit('Fixture already exists; use another private directory.')
        s={'base_url':base,'password':'x'+secrets.token_urlsafe(32),'temenos_password':'x'+secrets.token_urlsafe(32)}
        private_json(root/'secret.json',s)
        config='[global]\nserver_name = "agora.test"\ndatabase_path = "/data"\naddress = "0.0.0.0"\nport = 8008\nallow_federation = false\ntrusted_servers = []\nallow_registration = false\nlog = "off"\nemergency_password = '+json.dumps(s['password'])+'\n'
        fd=os.open(root/'config.toml',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'w') as f:f.write(config)
        print('Private config initialized; start the isolated compose project.');return
    s=json.loads((root/'secret.json').read_text());assert s['base_url']==base
    def req(method,path,data=None,token=None):
        r=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),method=method,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
        try:
            with urllib.request.urlopen(r,timeout=15) as response:return json.load(response)
        except urllib.error.HTTPError as e:return json.load(e)
    def login(user,password):
        result=req('POST','/_matrix/client/v3/login',{'type':'m.login.password','identifier':{'type':'m.id.user','user':user},'password':password})
        if 'access_token' not in result:raise RuntimeError('Fixture login failed: '+result.get('errcode','unknown'))
        return result['access_token']
    bot=login('@conduit:agora.test',s['password'])
    rooms=req('GET','/_matrix/client/v3/joined_rooms',token=bot)['joined_rooms'];assert len(rooms)==1
    s['room']=rooms[0];room=urllib.parse.quote(s['room'],safe='')
    def command(body):
        cursor=req('GET','/_matrix/client/v3/sync?timeout=0',token=bot)['next_batch']
        event=req('PUT',f'/_matrix/client/v3/rooms/{room}/send/m.room.message/{uuid.uuid4().hex}',{'msgtype':'m.text','body':body},bot)['event_id']
        for _ in range(30):
            response=req('GET','/_matrix/client/v3/sync?timeout=1000&since='+urllib.parse.quote(cursor),token=bot);cursor=response['next_batch']
            for joined in response.get('rooms',{}).get('join',{}).values():
                for e in joined.get('timeline',{}).get('events',[]):
                    c=e.get('content',{})
                    if e.get('sender')=='@conduit:agora.test' and c.get('m.relates_to',{}).get('m.in_reply_to',{}).get('event_id')==event:return c.get('body','')
        raise RuntimeError('No correlated admin reply')
    if args.action=='bootstrap':
        response=command('!admin users create-user -- temenos '+s['temenos_password'])
        if 'Created user @temenos:agora.test with password' not in response and 'Username is not available.' not in response:raise RuntimeError('Fixture account creation failed; response withheld.')
        command('!admin users make-user-admin @temenos:agora.test')
        s['temenos_token']=login('temenos',s['temenos_password'])
        req('POST','/_matrix/client/v3/join/'+room,{},s['temenos_token']);private_json(root/'secret.json',s)
        print('Temenos admin account ready; credentials remain in private fixture.');return
    name='proof'+secrets.token_hex(6);p1='x'+secrets.token_urlsafe(24);p2='x'+secrets.token_urlsafe(24)
    assert f'Created user @{name}:agora.test with password' in command(f'!admin users create-user {name} {p1}')
    assert 'Username is not available.' in command(f'!admin users create {name} {p1}')
    stable='ownership-'+name
    send=f'/_matrix/client/v3/rooms/{room}/send/m.room.message/{stable}'
    ownname=name+'own'
    original=req('PUT',send,{'msgtype':'m.text','body':f'!admin users create-user {ownname} {p1}'},s['temenos_token'])['event_id']
    replay=req('PUT',send,{'msgtype':'m.text','body':f'!admin users create-user {ownname} {p2}'},s['temenos_token'])['event_id']
    assert original==replay
    for _ in range(30):
        history=req('GET',f'/_matrix/client/v3/rooms/{room}/messages?dir=b&limit=100',token=s['temenos_token'])
        replies=[e for e in history.get('chunk',[]) if e.get('sender')=='@conduit:agora.test' and e.get('content',{}).get('m.relates_to',{}).get('m.in_reply_to',{}).get('event_id')==original]
        if replies:break
        time.sleep(.1)
    assert len(replies)==1 and f'Created user @{ownname}:agora.test with password' in replies[0]['content']['body']
    context=req('GET',f'/_matrix/client/v3/rooms/{room}/context/'+urllib.parse.quote(original,safe='')+'?limit=20',token=s['temenos_token'])
    assert any(e.get('content',{}).get('m.relates_to',{}).get('m.in_reply_to',{}).get('event_id')==original for e in context.get('events_after',[]))
    token=login(name,p1)
    assert req('PUT',f'/_matrix/client/v3/profile/@{name}:agora.test/displayname',{'displayname':'Fíxture Wanax'},token)=={}
    assert req('GET',f'/_matrix/client/v3/profile/@{name}:agora.test/displayname')['displayname']=='Fíxture Wanax'
    assert f'Successfully reset the password for user @{name}:agora.test:' in command(f'!admin users reset-password {name} {p2}')
    old=req('POST','/_matrix/client/v3/login',{'type':'m.login.password','identifier':{'type':'m.id.user','user':name},'password':p1});assert old.get('errcode')=='M_FORBIDDEN'
    login(name,p2)
    assert f'User @{name}:agora.test has been deactivated' in command(f'!admin users deactivate @{name}:agora.test')
    assert 'Username is not available.' in command(f'!admin users create {name} {p1}')
    dash='-'+name
    assert f'Created user @{dash}:agora.test with password' in command(f'!admin users create-user -- {dash} {p1}')
    assert f'Successfully reset the password for user @{dash}:agora.test:' in command(f'!admin users reset-password -- {dash} {p2}')
    assert f'User @{dash}:agora.test has been deactivated' in command(f'!admin users deactivate -- @{dash}:agora.test')
    print('PASS: create-user alias, reply correlation, active/deactivated collisions, display name, password rotation, deactivate, stable transaction replay and historical ownership proof.')

if __name__=='__main__':
    try:main()
    except Exception as e:raise SystemExit('Rigg failed: '+type(e).__name__+' (raw details withheld to protect credentials)') from None
