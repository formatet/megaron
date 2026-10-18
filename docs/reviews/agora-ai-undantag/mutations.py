#!/usr/bin/env python3
"""Physical compiling mutations; assertion RED, exact restoration, fresh GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
server=lambda name:['tools/gotest.sh','./cmd/server','-run',name,'-v']
api=['tools/gotest.sh','./api/handlers','-run','TestAgoraDisabledPendingAndUpstreamFailuresAreExplicitAndRedacted','-v']
cli=lambda name:['go','-C','server','test','./cmd/keryx','-run',name,'-count=1','-v']
js=['node','--test','web/static/js/megaron/ui/account_window.test.mjs']
cases=[
 ('pass-exemption', 'server/cmd/server/agora.go', 'WHERE NOT p.agora_exempt AND p.agora_localpart IS NULL', 'WHERE p.agora_localpart IS NULL', server('TestAgoraExemptPassSkipsReconcile'), 'TestAgoraExemptPassSkipsReconcile'),
 ('direct-exemption', 'server/cmd/server/agora.go', 'if exempt || localpart != "" {', 'if localpart != "" {', server('TestAgoraExemptDirectAndAccountLifecycle'), 'TestAgoraExemptDirectAndAccountLifecycle'),
 ('account-exemption', 'server/cmd/server/agora.go', 'if exempt {\n\t\treturn handlers.AgoraAccount', 'if false {\n\t\treturn handlers.AgoraAccount', server('TestAgoraExemptDirectAndAccountLifecycle'), 'TestAgoraExemptDirectAndAccountLifecycle'),
 ('password-exemption', 'server/cmd/server/agora.go', 'if exempt {\n\t\treturn handlers.AgoraPassword', 'if false {\n\t\treturn handlers.AgoraPassword', server('TestAgoraExemptDirectAndAccountLifecycle'), 'TestAgoraExemptDirectAndAccountLifecycle'),
 ('backfill-gpt', 'server/db/migrations/183_agora_exempt.up.sql', "OR username LIKE 'gpt-%'", "OR username LIKE 'never-gpt-%'", server('TestAgoraExemptMigrationBackfillAndDefault'), 'TestAgoraExemptMigrationBackfillAndDefault'),
 ('api-forbidden', 'server/api/handlers/agora.go', 'if errors.Is(err, ErrAgoraExempt) {', 'if false {', api, 'TestAgoraDisabledPendingAndUpstreamFailuresAreExplicitAndRedacted'),
 ('cli-exempt-state', 'server/cmd/keryx/cmd_agora.go', 'case status.State == "exempt":', 'case status.State == "missing":', cli('TestAgoraStatusNeverRequestsPassword'), 'TestAgoraStatusNeverRequestsPassword'),
 ('cli-exempt-password', 'server/cmd/keryx/cmd_agora.go', 'status == http.StatusForbidden', 'status == http.StatusGone', cli('TestAgoraExemptPasswordHasNoRetryAdvice'), 'TestAgoraExemptPasswordHasNoRetryAdvice'),
 ('web-exempt-state', 'web/static/js/megaron/ui/account_window.js', "account.state === 'exempt'", "account.state === 'missing'", js, 'exempt account'),
 ('web-exempt-password', 'web/static/js/megaron/ui/account_window.js', 'response.status === 403', 'response.status === 418', js, 'newly exempt password'),
]
cases=cases[int(sys.argv[1]):] if len(sys.argv)>1 else cases
results=[]
for name,relative,old,new,cmd,named in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(label,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+label+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-3000:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log) and '[build failed]' not in log,(name,log[-3000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named assertion RED -> exact bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
