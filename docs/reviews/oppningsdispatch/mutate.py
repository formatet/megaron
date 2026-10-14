#!/usr/bin/env python3
"""Real source mutations, named failure and exact byte restoration on fresh PG16."""
import pathlib, subprocess, os
HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parents[2]
cases=[
 ('missing-archive', [('server/api/handlers/opening_dispatch.go', 'VALUES($1,$2,$3,$4,$5) RETURNING id', "SELECT $1,$2,$3,$4,$5 WHERE false RETURNING id")], 'TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy'),
 ('hardcoded-grace', [('server/internal/kharis/tick.go', 'const foundingGraceTicks = 60', 'const foundingGraceTicks = 61'),('server/api/handlers/opening_dispatch.go','kharis.FoundingGraceTicks(),','60,')], 'TestOpeningDispatchFactsFromActiveRules'),
]
for name, edits, test in cases:
 originals={ROOT/rel:(ROOT/rel).read_bytes() for rel,_,_ in edits}
 try:
  for rel,before,after in edits:
   path=ROOT/rel;source=originals[path].decode();assert source.count(before)==1
   # Keep import used when mutating the consumer to a stale literal.
   if name=='hardcoded-grace' and rel.endswith('opening_dispatch.go'):
    after='kharis.FoundingGraceTicks()-1,'
   path.write_text(source.replace(before,after))
  result=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run','^'+test+'$','-v'],cwd=ROOT,env={**os.environ,'GOTEST_PORT':'57411'},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (HERE/(name+'-red.log')).write_text(result.stdout)
  assert result.returncode!=0 and '--- FAIL: '+test in result.stdout and '[build failed]' not in result.stdout,result.stdout
 finally:
  for path,original in originals.items():path.write_bytes(original)
 for path,original in originals.items():assert path.read_bytes()==original
 result=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run','^'+test+'$','-v'],cwd=ROOT,env={**os.environ,'GOTEST_PORT':'57411'},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (HERE/(name+'-restored-green.log')).write_text(result.stdout)
 assert result.returncode==0,result.stdout
 print('PASS '+name+': named RED -> identical bytes -> fresh GREEN',flush=True)

# Archive and live watch share these formatters. Removing the last paragraph
# must fail the named client test rather than merely fail compilation.
client_cases=[
 ('web-missing-paragraph','web/static/js/megaron/ui/dispatch_window.js',
  "paragraphs.filter(p => typeof p === 'string')", "paragraphs.slice(0, -1).filter(p => typeof p === 'string')",
  ['node','--test','web/static/js/megaron/ui/dispatch_window.test.mjs'],
  'not ok 17 - opening dispatch preserves all archived paragraphs and escapes text'),
 ('keryx-missing-paragraph','server/cmd/keryx/cmd_notifications.go',
  'for _, paragraph := range body.Paragraphs {','for _, paragraph := range body.Paragraphs[:len(body.Paragraphs)-1] {',
  [str(ROOT/'tools/gotest.sh'),'./cmd/keryx','-run','^TestOpeningDispatchSummaryAndArchiveWatchDetails$','-v'],
  '--- FAIL: TestOpeningDispatchSummaryAndArchiveWatchDetails'),
]
for name,rel,before,after,command,failed in client_cases:
 path=ROOT/rel;original=path.read_bytes();source=original.decode();assert source.count(before)==1
 try:
  path.write_text(source.replace(before,after))
  result=subprocess.run(command,cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (HERE/(name+'-red.log')).write_text(result.stdout)
  assert result.returncode!=0 and failed in result.stdout and '[build failed]' not in result.stdout,result.stdout
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
 result=subprocess.run(command,cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (HERE/(name+'-restored-green.log')).write_text(result.stdout)
 assert result.returncode==0,result.stdout
 print('PASS '+name+': named RED -> identical bytes -> fresh GREEN',flush=True)
