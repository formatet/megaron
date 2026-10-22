#!/usr/bin/env python3
"""Physical shield and positive-net-warning regressions; fresh DB per arm."""
from pathlib import Path
import subprocess,json,hashlib
out=Path(__file__).resolve().parent;root=out.parents[2]
cases=[('shield',root/'server/internal/kharis/tick.go',b'if anchor.Tick < until {',b'if false {','TestWrathProtectionLastsSevenTicksAndDoesNotExtendOnRetry','tick10 stock25 want50'),('positive-net',root/'server/internal/kharis/transition_notices.go',b'if net < 0 {',b'if true {','TestWrathDeclinePositiveNetSuppressedAndRecoveryRearms','step0 warnings1 want0')]
restored={}
for name,p,old,new,test,assertion in cases:
 original=p.read_bytes();assert original.count(old)==1
 try:
  p.write_bytes(original.replace(old,new,1))
  with (out/f'mutation-{name}-red.log').open('w') as log:
   result=subprocess.run(['bash','tools/gotest.sh','./internal/kharis','-run','^'+test+'$'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
  text=(out/f'mutation-{name}-red.log').read_text();assert result.returncode!=0 and assertion in text and '--- FAIL: '+test in text,text
 finally:p.write_bytes(original)
 assert p.read_bytes()==original;restored[str(p.relative_to(root))]=hashlib.sha256(original).hexdigest()
with (out/'mutation-restored-green.log').open('w') as log:
 result=subprocess.run(['bash','tools/gotest.sh','./internal/kharis','./api/handlers','./cmd/keryx','-run','TestWrath|TestOwnerWrath|TestProvinceCultUpkeep|TestStatusDivineProtection'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert result.returncode==0,'restored tests failed'
(out/'mutation.json').write_text(json.dumps({'physical_red_cases':['shield','positive-net'],'byte_exact_restore':restored,'restored_green':True},indent=2)+'\n')
