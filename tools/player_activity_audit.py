#!/usr/bin/env python3
"""Read-only operator activity report; no message contents, credentials or public endpoint.
Usage: python3 tools/player_activity_audit.py OUTPUT.json
NULL means not observed since activity tracking was deployed. Authenticated
background polling counts; this is not proof of human attention. Login/token
creation is a separate column and is not used to invent last_seen_at.
"""
import json,pathlib,subprocess,sys
REMOTE = 'import pathlib,shlex,subprocess,json,urllib.request\nvals={}\nfor line in pathlib.Path(\'/opt/poleia/.env\').read_text().splitlines():\n line=line.strip()\n if line and not line.startswith(\'#\') and \'=\' in line:\n  k,v=line.split(\'=\',1)\n  if k.strip()==\'DATABASE_URL\':vals[k.strip()]=shlex.split(v)[0]\nq="SELECT json_build_object(\'worlds\',(SELECT json_agg(json_build_object(\'id\',id,\'name\',name,\'status\',status,\'tick\',current_tick,\'created_at\',created_at)) FROM worlds),\'players\',(SELECT json_agg(json_build_object(\'id\',id,\'username\',username,\'created_at\',created_at,\'last_seen_at\',to_jsonb(p)->\'last_seen_at\',\'latest_token_created\',(SELECT max(created_at) FROM refresh_tokens WHERE player_id=p.id),\'latest_messenger\',(SELECT json_build_object(\'id\',m.id,\'world_id\',m.world_id,\'sent_at\',m.sent_at,\'sent_tick\',m.sent_tick,\'kind\',m.kind,\'status\',m.status) FROM messengers m WHERE m.sender_id=p.id ORDER BY m.sent_at DESC LIMIT 1))) FROM players p));"\nr=subprocess.run([\'psql\',vals[\'DATABASE_URL\'],\'-XAt\',\'-v\',\'ON_ERROR_STOP=1\',\'-c\',q],capture_output=True,text=True)\nif r.returncode:raise SystemExit(\'read-only activity query failed\')\nprint(r.stdout)\n'
result=subprocess.run(['ssh','root@10.0.1.92','python3','-'],input=REMOTE,text=True,capture_output=True,check=True)
proof=json.loads(result.stdout)
output=pathlib.Path(sys.argv[1]);output.touch(mode=0o600,exist_ok=True);output.chmod(0o600)
output.write_text(json.dumps(proof,indent=2)+'\n')
for player in proof['players'] or []:
 print(player['username'], 'authenticated_contact=',player['last_seen_at'],'token_created=',player['latest_token_created'])
