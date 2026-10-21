#!/usr/bin/env python3
"""Prove that acknowledging a letter persists its receipt without sending it home."""
from pathlib import Path
import argparse,subprocess
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output-dir',type=Path,required=True);a=p.parse_args();a.output_dir.mkdir(parents=True,exist_ok=True)
root=Path(__file__).resolve().parents[1];source=root/'server/api/handlers/messenger_read.go';original=source.read_text();needle='read_at=COALESCE(m.read_at,$4)';assert original.count(needle)==1

def run(name,red):
 r=subprocess.run(['tools/gotest.sh','./api/handlers','-run','^TestMessengerReadReceiptIsAuthorizedIdempotentAndDoesNotReply$'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (a.output_dir/(name+'.log')).write_text(r.stdout)
 assert ('--- FAIL: TestMessengerReadReceiptIsAuthorizedIdempotentAndDoesNotReply' in r.stdout and r.returncode and 'build failed' not in r.stdout) if red else r.returncode==0,r.stdout
 print(name+(': named RED' if red else ': GREEN'),flush=True)
run('baseline',False)
try:
 source.write_text(original.replace(needle,'read_at=COALESCE(m.read_at,$4), status=\'returning\''))
 run('reading-sends-runner-home',True)
finally:source.write_text(original)
run('restored',False)
