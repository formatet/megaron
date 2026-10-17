#!/usr/bin/env python3
"""Physically revert the migrated rate; real assertions must fail on fresh DB."""
from pathlib import Path
import os,socket,subprocess,hashlib
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
p=ROOT/'server/db/migrations/182_hills_grain.up.sql';original=p.read_bytes()
assert original.count(b'rate_per_tick = 0.5')==1
cmd=['tools/gotest.sh','./internal/kharis','./api/handlers','-run','TestInlandFoodBalance_SevenOfTen|TestCreateMetropolis_InlandFoodSevenOfTen','-v']
def run(label):
 env={k:os.environ[k] for k in ('HOME','PATH')}
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
 r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/('mutation-'+label+'.log')).write_text(log);return r.returncode,log
try:
 p.write_bytes(original.replace(b'rate_per_tick = 0.5',b'rate_per_tick = 1.0 / 3.0'))
 code,log=run('red')
 assert code and '--- FAIL: TestInlandFoodBalance_SevenOfTen/hills' in log and '--- FAIL: TestCreateMetropolis_InlandFoodSevenOfTen' in log,log[-3000:]
finally:p.write_bytes(original)
code,log=run('restored');assert code==0,log[-3000:];assert p.read_bytes()==original
print('TWO named assertion RED -> exact migration bytes -> fresh GREEN; SHA256',hashlib.sha256(original).hexdigest())
