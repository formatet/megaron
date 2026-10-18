#!/usr/bin/env python3
from pathlib import Path
import hashlib,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
p=ROOT/'server/internal/province/eyes.go'
def run(label,expected,test='TestPlacementLiveEyesTransaction'):
 env={k:os.environ[k] for k in ('HOME','PATH')}
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
 r=subprocess.run(['tools/gotest.sh','./api/handlers','-run',test,'-v'],cwd=ROOT,env=env,capture_output=True,text=True)
 log=r.stdout+r.stderr;(OUT/(label+'.log')).write_text(log)
 if expected:assert r.returncode==0,log[-3000:]
 else:assert r.returncode and ('--- FAIL: '+test) in log and '[build failed]' not in log,log[-3000:]
for label,rows,items in ([] if '--logging-only' in __import__('sys').argv else [('unit','uRows','units'),('outbound','mRows','outbound'),('returning','rRows','returning')]):
 original=p.read_bytes();s=original.decode()
 old=f'\t\t\t{items} = append({items}, row)\n\t\t}}\n\t\t{rows}.Close()\n\t\t// Drain before interpolation: Queryer may be a single TX connection.\n\t\tfor _, row := range {items} {{'
 assert s.count(old)==1
 # Process the row inside its still-open reader; mark the retained slice used.
 new=f'\t\t\t{items} = append({items}, row)'
 s=s.replace(old,new)
 # Close the reader only after processing, recreating the original ordering.
 start=s.index(new);end=s.index('\n\t\t}\n\t}',start)
 s=s[:end]+s[end:].replace('\n\t\t}\n\t}',f'\n\t\t}}\n\t\t{rows}.Close()\n\t}}',1)
 try:p.write_text(s);run(label+'-red',False)
 finally:p.write_bytes(original)
 run(label+'-restored',True);assert p.read_bytes()==original
 print(label,'named RED -> exact bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)

p=ROOT/'server/api/handlers/settlement_placement.go'
for label,operation,field in [('log-place-cause','place','cause'),('log-unplace-cause','unplace','cause'),('log-pg-code','place','code')]:
 original=p.read_bytes();s=original.decode()
 if field=='cause':
  start=s.index('attrs := []any{"operation", "'+operation+'"');end=s.index('\n',start)
  line=s[start:end];assert line.count(', "err", err')==1
  s=s[:start]+line.replace(', "err", err','')+s[end:]
 else:
  assert s.count('"pg_code", pgErr.Code')==2
  s=s.replace('"pg_code", pgErr.Code','"pg_code", ""')
 try:p.write_text(s);run(label+'-red',False,'TestPlacementInternalErrorLogsDatabaseCause')
 finally:p.write_bytes(original)
 run(label+'-restored',True,'TestPlacementInternalErrorLogsDatabaseCause');assert p.read_bytes()==original
 print(label,'named RED -> exact bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
