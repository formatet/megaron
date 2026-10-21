#!/usr/bin/env python3
"""Prove recipient contact depends on a delivered incoming city endpoint."""
from pathlib import Path
import argparse
import subprocess
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--output-dir',type=Path,required=True)
a=p.parse_args();a.output_dir.mkdir(parents=True,exist_ok=True)
root=Path(__file__).resolve().parents[1]
source=root/'server/internal/province/contacts.go';original=source.read_text()
needle='dest.owner_id=$2 AND NOT m.withdrawn';assert original.count(needle)==1

def run(name,red):
 r=subprocess.run(['tools/gotest.sh','./api/handlers','-run','^TestIncomingContactRemembersOnlyOriginAfterPhysicalDelivery$'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (a.output_dir/(name+'.log')).write_text(r.stdout)
 assert ('--- FAIL: TestIncomingContactRemembersOnlyOriginAfterPhysicalDelivery' in r.stdout and r.returncode and 'build failed' not in r.stdout) if red else r.returncode==0,r.stdout
 print(name+(': named RED' if red else ': GREEN'),flush=True)
run('baseline',False)
try:
 source.write_text(original.replace(needle,'dest.owner_id=$2 AND false AND NOT m.withdrawn'))
 run('one-way-contact',True)
finally:source.write_text(original)
run('restored',False)
