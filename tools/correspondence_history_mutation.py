#!/usr/bin/env python3
"""Prove old received letters cannot depend on an actionable inbox filter."""
from pathlib import Path
import argparse,subprocess
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output-dir',type=Path,required=True);a=p.parse_args();a.output_dir.mkdir(parents=True,exist_ok=True)
root=Path(__file__).resolve().parents[1];source=root/'server/api/handlers/messenger.go';original=source.read_text();needle='r.URL.Query().Get("history") == "true"';assert original.count(needle)==1
try:
 source.write_text(original.replace(needle,'false'))
 r=subprocess.run(['tools/gotest.sh','./api/handlers','-run','^TestMessengerReadReceiptIsAuthorizedIdempotentAndDoesNotReply$'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (a.output_dir/'pending-inbox-only.log').write_text(r.stdout)
 assert r.returncode and '--- FAIL: TestMessengerReadReceiptIsAuthorizedIdempotentAndDoesNotReply' in r.stdout and 'build failed' not in r.stdout,r.stdout
 print('pending-inbox-only: named RED',flush=True)
finally:source.write_text(original)
