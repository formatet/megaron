#!/usr/bin/env python3
"""Fresh migrated PG, actual Go audit and read-only Python tool; destroys own PG."""
import json,os,pathlib,re,shlex,subprocess
here=pathlib.Path(__file__).resolve().parent;root=here.parents[2]
clean={k:os.environ[k] for k in ('HOME','PATH')}
result=subprocess.run([str(root/'tools/gotest.sh'),'./internal/combat','-run','TestSilverAuditV2|TestSilverMiningCredit','-v'],cwd=root,env={**clean,'GOTEST_KEEP':'1'},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(here/'reader-fresh.log').write_text(result.stdout)
match=re.search(r'containern står kvar: (megaron-gotest-[0-9]+)  \((postgres://[^)]+)\)',result.stdout)
assert match,result.stdout
name,dsn=match.groups()
try:
 assert result.returncode==0,result.stdout
 raw=re.search(r'AUDIT_V2_JSON=(\{[^\n]+\})',result.stdout)
 assert raw,result.stdout
 payload=json.loads(raw.group(1));(here/'actual-payload.json').write_text(json.dumps(payload,indent=2)+'\n')
 psql=['docker','exec','-i',name,'psql','-U','postgres','-d','megaron_test']
 env={**clean,'ALFA_TEST_PSQL_CMD':shlex.join(psql),'ALFA_AUDIT_JSON':str(here/'actual-payload.json')}
 tested=subprocess.run(['python3',str(root/'tools/alfa_economy_test.py')],env=env,cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (here/'reader-tests.log').write_text(tested.stdout)
 assert tested.returncode==0 and 'skipped' not in tested.stdout,tested.stdout
 world=subprocess.check_output([*psql,'-tAc',"SELECT world_id FROM events WHERE event_type='SilverAuditV2' ORDER BY id DESC LIMIT 1"],text=True).strip()
 report=subprocess.check_output(['python3',str(root/'tools/alfa_economy.py'),'--psql-cmd',shlex.join(psql),'--world',world],env=clean,cwd=root,text=True)
 (here/'actual-economy-report.md').write_text(report)
 assert 'SilverAuditV2' in report and '735.00' in report,report
 version=subprocess.check_output([*psql,'-tAc','SELECT version,dirty FROM schema_migrations'],text=True).strip()
 assert version=='179|f',version
 print('PASS fresh PG179 clean; actual V2 JSON + read-only DB; seven reader tests, no skips; complete economy report')
finally:
 subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
