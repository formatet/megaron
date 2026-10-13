#!/usr/bin/env python3
"""Physical mutations, separate fresh PG per arm, exact byte restoration."""
import hashlib,pathlib,subprocess
here=pathlib.Path(__file__).resolve().parent
root=here.parents[2]
target=root/'server/api/handlers/unit_march_silver.go'
original=target.read_bytes()
for name,before,after in [('field-circulates',b'after = gross',b'after = net'),('cargo-missing',b'selected[*cargo] = true',b'// deliberately omitted cargo')]:
 assert original.count(before)==1,name
 try:
  target.write_bytes(original.replace(before,after))
  result=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^TestMarchSilverForecastMatchesActualTick$'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (here/(name+'-red.log')).write_text(result.stdout)
  assert result.returncode and 'FAIL: TestMarchSilverForecastMatchesActualTick' in result.stdout,result.stdout
 finally:
  target.write_bytes(original)
 assert target.read_bytes()==original
 result=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^TestMarchSilverForecastMatchesActualTick$'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (here/(name+'-restored-green.log')).write_text(result.stdout)
 assert result.returncode==0,result.stdout
 print(name+' RED → byte-restored GREEN; sha256 '+hashlib.sha256(original).hexdigest(),flush=True)
