#!/usr/bin/env python3
"""K1: fresh PG/Redis, normal register/join/found, labelled delivered-letter
fixtures, actual Keryx and Firefox queue/reply/decline/paging at 1280 and 390.
Usage: python3 tools/correspondence_live.py OUT [EXPECTED_BUILD]
Builds current sources in a clean environment. Never touches shared services.
The browser_queue helper can also be imported by another owned-runtime proof.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def main():
    out = Path(sys.argv[1]).resolve()
    out.mkdir(parents=True, exist_ok=True)
    prefix = 'megaron-k4-' + secrets.token_hex(4)
    containers, proc = [], None

    def run(*args, **kw):
        return subprocess.check_output(args, text=True, **kw).strip()

    def launch(kind, image, args):
        name = prefix + '-' + kind
        internal = '5432' if kind == 'pg' else '6379'
        run('docker', 'run', '-d', '--rm', '--name', name, '-p', '127.0.0.1::' + internal, *args, image)
        containers.append(name)
        ports = json.loads(run('docker', 'inspect', name))[0]['NetworkSettings']['Ports']
        return int(ports[internal + '/tcp'][0]['HostPort'])

    try:
        with tempfile.TemporaryDirectory(prefix='correspondence-live-') as tmpdir:
            tmp = Path(tmpdir)
            clean = {k: os.environ[k] for k in ('HOME', 'PATH')}
            clean['CGO_ENABLED'] = '0'
            commit = sys.argv[2] if len(sys.argv) > 2 else run('git', '-C', str(ROOT), 'rev-parse', '--short=7', 'HEAD') + '+k4'
            hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
                      for directory in ('server/api/handlers', 'server/cmd/keryx', 'web/static/js/megaron/ui/drawers')
                      for p in (ROOT / directory).glob('*') if p.is_file() and ('messenger' in p.name or 'correspondence' in p.name or 'diplomacy' in p.name)}
            run('go', 'build', '-ldflags', '-X main.buildCommit=' + commit, '-o', str(tmp / 'temenos'), './cmd/server', cwd=ROOT / 'server', env=clean)
            run('go', 'build', '-o', str(tmp / 'keryx'), './cmd/keryx', cwd=ROOT / 'server', env=clean)
            print('BUILD_COMPLETE', commit, flush=True)
            pg = launch('pg', os.environ.get('CORRESPONDENCE_PG_IMAGE', 'postgres:16-alpine'), ['-e', 'POSTGRES_PASSWORD=k1-proof', '-e', 'POSTGRES_DB=k1_proof'])
            redis = launch('redis', 'redis:7-alpine', [])
            for _ in range(80):
                if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', '-q'], capture_output=True).returncode == 0:
                    break
                time.sleep(.25)
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            base = f'http://127.0.0.1:{port}'
            env = {**clean, 'DATABASE_URL': f'postgres://postgres:k1-proof@localhost:{pg}/k1_proof?sslmode=disable', 'REDIS_URL': f'localhost:{redis}',
                   'JWT_SECRET': secrets.token_urlsafe(40), 'PORT': str(port), 'TICK_SECONDS': '3600', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
                   'WORLD_NAME': 'Correspondence K1 proof', 'POLEIA_WORLD_START_WANAXES': '1', 'STATIC_DIR': str(ROOT / 'web/static'),
                   'TEMPLATE_DIR': str(ROOT / 'web/templates'), 'CHRONICLE_DIR': str(tmp / 'chronicles'), 'REPORTS_DIR': str(tmp / 'reports')}
            assert not (ROOT / 'server/.env').exists()
            proc = subprocess.Popen([str(tmp / 'temenos')], cwd=ROOT / 'server', env=env, stdout=(out / 'server.log').open('w'), stderr=subprocess.STDOUT)

            def api(path, method='GET', data=None, token=None):
                req = urllib.request.Request(base + path, method=method, data=None if data is None else json.dumps(data).encode(),
                                            headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer ' + token} if token else {})})
                return json.load(urllib.request.urlopen(req, timeout=30))

            for _ in range(120):
                try:
                    health = api('/healthz')
                    break
                except Exception:
                    if proc.poll() is not None:
                        raise RuntimeError('Server exited; see server.log')
                    time.sleep(.5)
            else:
                raise RuntimeError('Server timeout')
            latest = max(int(p.name.split('_')[0]) for p in (ROOT / 'server/db/migrations').glob('*.up.sql'))
            assert health['commit'] == commit and health['migration'] == latest, health
            world = api('/api/v1/worlds')[0]['id']
            wp = '/api/v1/worlds/' + world

            def sql(q):
                return run('docker', 'exec', containers[0], 'psql', '-U', 'postgres', '-d', 'k1_proof', '-At', '-v', 'ON_ERROR_STOP=1', '-c', q)

            def actor():
                token = api('/api/v1/auth/register', 'POST', {'username': 'Correspondence-' + secrets.token_hex(4), 'password': secrets.token_urlsafe(30)})['access_token']
                api(wp + '/join', 'POST', {}, token)
                host = next(u for u in api(wp + '/units', token=token)['units'] if u['type'] == 'nomadic_host')
                land = sql(f"SELECT landmass_id FROM map_tiles WHERE world_id='{world}' AND q={host['q']} AND r={host['r']}")
                return {'token': token, 'host': host, 'landmass': land, 'pid': api('/api/v1/auth/me', token=token)['id']}

            recipient = actor()
            # Landmass IDs do not prove a courier-land route: rivers and
            # mountains may split one landmass. Select a dry connected pair.
            tiles=json.loads(sql(f"SELECT json_agg(t) FROM (SELECT q,r,terrain FROM map_tiles WHERE world_id='{world}')t"))
            dry={(t['q'],t['r']) for t in tiles if t['terrain'] not in ('coastal_sea','deep_sea','river','mountain_limestone','mountain_red')}
            origin=(recipient['host']['q'],recipient['host']['r']);connected={origin};pending=[origin]
            while pending:
                q,r=pending.pop()
                for dq,dr in ((1,0),(1,-1),(0,-1),(-1,0),(-1,1),(0,1)):
                    point=(q+dq,r+dr)
                    if point in dry and point not in connected:connected.add(point);pending.append(point)
            for _ in range(40):
                sender = actor()
                if (sender['host']['q'],sender['host']['r']) in connected:
                    break
            else:
                raise AssertionError('No ordinary pair with a dry courier route')
            for who, name in [(recipient, 'K2 Nostos'), (sender, 'K2 Kyme')]:
                who['sid'] = api(wp + '/founding/settle', 'POST', {'name': name}, who['token'])['settlement_id']
                who['province'] = next(p for p in api(wp + '/provinces', token=who['token']) if p.get('settlement_id') == who['sid'])
            # Explicit visibility fixture, after normal register/join/found.
            # Travel is not under test here; delivery is staged below and labelled.
            for a,b in [(sender,recipient),(recipient,sender)]:
                sql(f"INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('{world}','{a['pid']}','{b['province']['id']}') ON CONFLICT DO NOTHING")
            endpoint=wp+'/settlements/'+sender['sid']+'/messengers'
            first=api(endpoint,'POST',{'destination_id':recipient['sid'],'message':'Copper for the winter voyage','subject':'Winter copper'},sender['token'])
            second=api(endpoint,'POST',{'destination_id':recipient['sid'],'message':'A separate grain negotiation','subject':'Spring grain'},sender['token'])
            case=first['case_id']
            assert second['case_id']!=case
            assert api(wp+'/correspondence/cases',token=recipient['token'])['items']==[], 'Outbound letter visible before arrival'
            # Explicit stock fixture before genuine offer/accept; no trade state is fabricated.
            sql(f"UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=current_world_tick() WHERE settlement_id IN('{sender['sid']}','{recipient['sid']}') AND good_key IN('grain','silver','copper');")
            offer=api(endpoint,'POST',{'destination_id':recipient['sid'],'message':'Two copper for five silver','subject':'Accepted copper','trade_offer':{'kind':'sell','offer_good':'copper','offer_qty':2,'want_silver':5}},sender['token'])
            for letter in (first,second,offer):
                sql(f"UPDATE messengers SET status='delivered',arrives_at=now()-interval '1 second' WHERE id='{letter['id']}'; INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES('{recipient['sid']}','province','MessengerArrived','{{\"messenger_id\":\"{letter['id']}\"}}','{world}');")
            accepted=api(wp+'/messengers/'+offer['id']+'/trade-accept','POST',{},recipient['token'])
            assert len(api(wp+'/correspondence/cases/'+case+'/memory',token=recipient['token'])['transactions'])==1
            assert api(wp+'/correspondence/cases/'+case+'/memory',token=sender['token'])['transactions']==[], 'Remote acceptance leaked before return'
            (out/'accepted.json').write_text(json.dumps(accepted,indent=2))
            config=tmp/'config.json'
            config.write_text(json.dumps({'server':base,'token':recipient['token'],'world_id':world,'province_id':recipient['province']['id']}));config.chmod(0o600)
            def cli(*args):
                return run(str(tmp/'keryx'),*args,env={**clean,'POLEIA_CONFIG':str(config)})
            (out/'cli-list.txt').write_text(cli('correspondence'))
            raw=cli('correspondence','--case',case,'--json'); detail=json.loads(raw)
            assert detail['items'][0]['message_text']=='Copper for the winter voyage'
            (out/'cli-detail.json').write_text(raw)
            assert len(api(wp+'/correspondence/cases',token=recipient['token'])['items'])==3
            errors=[]
            with sync_playwright() as pw:
                browser=pw.firefox.launch()
                for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
                    ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=label=='mobile')
                    ctx.add_cookies([{'name':'poleia_token','value':recipient['token'],'url':base}])
                    ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(recipient['token'])+')')
                    page=ctx.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
                    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
                    for brief in page.locator('.lb-dismiss').all():
                        if brief.is_visible():brief.click()
                    page.locator('#trig-diplomacy').click();page.locator('[data-tab="archive"]').click()
                    root=page.locator('#dtab-archive');root.locator('[data-case-open="'+case+'"]').wait_for()
                    (out/(label+'-cases-text.txt')).write_text(root.inner_text())
                    assert 'spring grain' in root.inner_text().lower() and 'winter copper' in root.inner_text().lower(), root.inner_text()
                    page.screenshot(path=str(out/(label+'-cases.png')))
                    root.locator('[data-case-open="'+case+'"]').click();root.locator('[data-archive-letter]').first.wait_for()
                    received=root.locator('[data-archive-letter="'+first['id']+'"] > details')
                    if received.get_attribute('open') is None:received.locator(':scope > summary').click()
                    assert 'Copper for the winter voyage' in root.inner_text() and 'A separate grain negotiation' not in root.inner_text()
                    assert root.evaluate('(e)=>e.scrollWidth<=e.clientWidth'), 'Horizontal overflow'
                    page.screenshot(path=str(out/(label+'-detail.png')))
                    memory=root.locator('[data-case-memory]');memory.get_by_text('Memory with this Wanax',exact=True).wait_for()
                    assert '2 copper for 5 silver' in memory.inner_text() and 'Accepted copper' in memory.inner_text()
                    citation=root.locator('[data-citation-letter="'+first['id']+'"]')
                    if label=='desktop':
                        citation.locator('..').locator(':scope > summary').click()
                        citation.locator('textarea').fill('winter voyage')
                        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/citations')) as kept:
                            citation.locator('button').click()
                        assert kept.value.status in (200,201),kept.value.text()
                        citation.get_by_text('Kept as your private note.',exact=False).wait_for()
                        memory.get_by_text('winter voyage',exact=True).wait_for()
                        memory.scroll_into_view_if_needed();page.screenshot(path=str(out/'desktop-memory.png'))
                        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/citations')) as repeated:
                            citation.locator('button').click()
                        assert repeated.value.status in (200,201)
                        note_record=memory.locator('article').filter(has=page.get_by_text('winter voyage',exact=True))
                        with page.expect_response(lambda r:r.request.method=='GET' and r.url.split('?')[0].endswith('/letters/'+first['id'])) as source_letter:
                            note_record.locator('[data-memory-case]').click()
                        assert source_letter.value.status==200
                        source_payload=source_letter.value.json()
                        assert len(source_payload['items'])==1 and source_payload['items'][0]['id']==first['id']
                        root.locator('[data-citation-letter="'+first['id']+'"]').wait_for(state='attached')
                    else:
                        memory.get_by_text('winter voyage',exact=True).wait_for()
                        memory.scroll_into_view_if_needed();page.screenshot(path=str(out/'mobile-memory.png'))
                        with page.expect_response(lambda r:r.request.method=='DELETE' and '/citations/' in r.url) as removed:
                            memory.locator('[data-citation-delete]').first.click()
                        assert removed.value.status==204
                        memory.get_by_text('No private passages on this page.',exact=True).wait_for()
                    if label=='desktop':
                        form=root.locator('[data-case-compose]');form.locator('textarea').fill('Keep this in the copper matter')
                        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/messengers')) as dispatched:
                            form.locator('button').click()
                        assert dispatched.value.status==201,dispatched.value.text()
                        sent=dispatched.value.json();assert sent['case_id']==case
                        form.get_by_text('Letter dispatched in this case.',exact=False).wait_for()
                        (out/'web-send.json').write_text(json.dumps(sent,indent=2))
                    ctx.close()
                browser.close()
            assert not errors,errors
            assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}' AND read_at IS NOT NULL"))==0, 'Archive listing/opening marked letters read'
            # Actual CLI follows up the same case; text fields are user input.
            sent_cli=json.loads(cli('message','--to','K2 Kyme','--case',case,'--text','Keryx keeps the case identity','--json'))
            assert sent_cli['case_id']==case
            (out/'cli-send.json').write_text(json.dumps(sent_cli,indent=2))
            kept_cli=json.loads(cli('correspondence-cite','--case',case,'--letter',first['id'],'--quote','winter voyage','--json'))
            source_cli=json.loads(cli('correspondence','--case',case,'--letter',first['id'],'--json'))
            assert len(source_cli['items'])==1 and source_cli['items'][0]['id']==first['id']
            (out/'source-cli.json').write_text(json.dumps(source_cli,indent=2))
            removed_cli=json.loads(cli('correspondence-cite-remove','--case',case,'--id',kept_cli['id'],'--json'))
            assert removed_cli=={},'DELETE204 CLI output must remain JSON'
            kept_cli=json.loads(cli('correspondence-cite','--case',case,'--letter',first['id'],'--quote','winter voyage','--json'))
            memory_cli=json.loads(cli('correspondence-memory','--case',case,'--json'))
            assert len(memory_cli['transactions'])==1 and len(memory_cli['citations'])==1
            assert api(wp+'/correspondence/cases/'+case+'/memory',token=sender['token'])['citations']==[], 'Private passage leaked to counterpart'
            sql(f"UPDATE messengers SET status='arrived' WHERE id='{offer['id']}';")
            assert len(api(wp+'/correspondence/cases/'+case+'/memory',token=sender['token'])['transactions'])==1
            (out/'memory-cli.json').write_text(json.dumps(memory_cli,indent=2))
            before=api(wp+'/correspondence/cases/'+case,token=recipient['token'])
            (out/'archive-before.json').write_text(json.dumps(before,indent=2))
            # Explicit ownership/name/deletion fixtures test secrecy/retention,
            # not a claim that a conquest happened through gameplay.
            successor=actor()
            sql(f"UPDATE players SET wanax_name='Renamed ruler' WHERE id='{sender['pid']}'; UPDATE settlements SET name='Renamed port' WHERE id='{sender['sid']}'; UPDATE settlements SET owner_id='{successor['pid']}' WHERE id='{recipient['sid']}';")
            after=api(wp+'/correspondence/cases/'+case,token=recipient['token'])
            assert after==before, 'Historical snapshots changed with current names/owners'
            assert api(wp+'/correspondence/cases',token=successor['token'])['items']==[]
            assert all(m['id'] not in [first['id'],second['id']] for m in api(wp+'/messengers/inbox?history=true',token=successor['token']))
            sql(f"BEGIN; UPDATE provinces SET controller_id=NULL WHERE controller_id='{sender['sid']}'; UPDATE player_world_records SET settlement_id=NULL WHERE settlement_id='{sender['sid']}'; DELETE FROM buildings WHERE settlement_id='{sender['sid']}'; DELETE FROM build_queue WHERE settlement_id='{sender['sid']}'; DELETE FROM settlements WHERE id='{sender['sid']}'; COMMIT;")
            archived=api(wp+'/correspondence/cases/'+case,token=recipient['token'])
            assert archived==before, 'City deletion deleted private archive'
            retained=api(wp+'/correspondence/cases/'+case+'/memory',token=recipient['token'])
            assert len(retained['transactions'])==1 and len(retained['citations'])==1, 'City deletion lost memory'
            (out/'archive-after-deletion.json').write_text(json.dumps(archived,indent=2))
            (out/'proof.json').write_text(json.dumps({'health':health,'world':world,'source_hashes':hashes,'result':'PASS','browser_errors':errors,'fixtures':'Normal register/join/found; explicit scouted contact + delivered states/events; real HTTP sends/native archive and continuation/actual CLI; explicit rename/owner/delete mutations.'},indent=2))
            print('PASS',health,'actual accepted trade, physical return visibility, native citation+dedup+mobile delete, CLI memory/cite, counterpart isolation and deletion retention')
    finally:
        if proc is not None:
            proc.terminate();proc.wait(timeout=20)
        for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)

if __name__=='__main__':main()
