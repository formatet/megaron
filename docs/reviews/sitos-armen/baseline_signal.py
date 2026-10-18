#!/usr/bin/env python3
"""Old physical kharis predicate: semantic RED and no-projection-side-effects proof."""
from pathlib import Path
import os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent;p=ROOT/'server/internal/kharis/tick.go';original=p.read_bytes()
def run(pattern,label,red=False):
 env={k:os.environ[k] for k in ('HOME','PATH')}
 with socket.socket() as s:s.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(s.getsockname()[1])
 r=subprocess.run(['tools/gotest.sh','./internal/kharis','-run',pattern,'-v'],cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/label).write_text(log)
 assert (r.returncode!=0 if red else r.returncode==0),(pattern,log)
 if red:assert '--- FAIL: TestStarvationSignal_NoFalseAuditAfterPaidUpkeep' in log and '[build failed]' not in log,log
try:
 p.write_bytes(subprocess.check_output(['git','-C',str(ROOT),'show','51e805a4:server/internal/kharis/tick.go']))
 run('TestStarvationSignal_NoFalseAuditAfterPaidUpkeep','baseline.log',True)
 run('TestStarvationSignal_AuditHasNoStateEffects','audit-before.log')
finally:p.write_bytes(original)
run('TestStarvationSignal_AuditHasNoStateEffects','audit-after.log')
assert p.read_bytes()==original
print('physical old predicate named RED; audit before/after no projection changes; exact restoration')
