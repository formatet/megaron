#!/usr/bin/env python3
"""Falsify inventory instruments using real controls and isolated temp files."""
import json
import gzip
import os
import pathlib
import subprocess
import tempfile

import analyze

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
ENV = {k: os.environ[k] for k in ('HOME','PATH','USER','LANG') if k in os.environ}
ENV.update(GOMAXPROCS='2',GOMEMLIMIT='1536MiB')
AST = json.loads(gzip.decompress((HERE/'raw/ast.json.gz').read_bytes()))
checks = []

def check(name, condition, evidence):
    assert condition, name
    checks.append(dict(name=name,passed=True,evidence=evidence))

sql = json.loads((HERE/'classified/sql-statements.json').read_text())
control = [g for g in sql if 'select distinct pos.q' in g['normalized']]
check('visibleOrigins SQL comments do not hide duplicate',len(control)==1 and
      {s['file'] for s in control[0]['sites']}=={'server/internal/capabilities/context.go','server/internal/province/contacts.go'},control)
unused = [s for s in AST['unused_parameters'] if s['function']=='dispatchShipReturnLeg' and s['value']=='arrivedID']
check('arrivedID unused in real return leg',len(unused)==1,unused)
check('SQL comments inside quoted values survive',analyze.sql_normalize("SELECT '--keep' -- drop\nFROM test") == "select '--keep' from test",'quote-aware comment removal')
# Actual negative arm: remove comment normalization and require the same real
# control to pass. The child assertion must fail, not silently yield zero.
source_sql=[s['value'] for s in AST['sql'] if (s['file'],s['line']) in
            {('server/internal/capabilities/context.go',193),('server/internal/province/contacts.go',11)}]
mutant_sql=subprocess.run(['python3','-c',
                          'import json,sys; values=json.load(sys.stdin); assert len(set(" ".join(v.lower().split()) for v in values))==1, "visibleOrigins control missed"'],
                         input=json.dumps(source_sql),env=ENV,text=True,capture_output=True)
check('SQL normalization mutant is red on visibleOrigins',mutant_sql.returncode==1,
      dict(exit=mutant_sql.returncode,stderr=mutant_sql.stderr))

dupl = pathlib.Path.home()/'.cache/megaron-steg4/bin/dupl'
source = 'server/internal/transport/carrier.go'
def scan(files):
    proc = subprocess.run([str(dupl),'-plumbing','-t','100','-files'],input='\n'.join(files)+'\n',
                          cwd=ROOT,env=ENV,text=True,capture_output=True,check=True)
    return proc.stdout

before = scan([source])
with tempfile.TemporaryDirectory(prefix='megaron-steg4-control-') as tmp:
    directory = pathlib.Path(tmp)
    copied = directory/'planted.go'
    copied.write_text((ROOT/source).read_text())
    planted = scan([source,str(copied)])
    check('dupl planted full production file hits',str(copied) in planted and source in planted,
          dict(source=source,planted_output=planted))
    copied.unlink()
    after = scan([source])
    check('removed planted copy disappears',before==after and str(copied) not in after,
          dict(before=before,after=after,temp_exists=copied.exists()))
    # AST object identity must respect shadowing and closure capture. Calls on
    # URL.Query are excluded from the DB metric, actual tx.Query is included.
    fixture = directory/'fixture.go'
    fixture.write_text('''package fixture
func unused(arrivedID int) { }
func used(id int) { _ = id }
func shadow(id int) { if true { id := 1; _ = id } }
func closure(id int) { f := func() int { return id }; _ = f }
''')
    proc = subprocess.run(['go','run','docs/reviews/steg4-svep/ast_measure.go',str(directory)],
                          cwd=ROOT,env=ENV,text=True,capture_output=True,check=True)
    measured = json.loads(proc.stdout)
    names = {(s['function'],s['value']) for s in measured['unused_parameters']}
    check('unused AST distinguishes used shadowed captured params',names=={('unused','arrivedID'),('shadow','id')},sorted(names))
    mutant=directory/'ast_measure_mutant.go'
    tool=(HERE/'ast_measure.go').read_text()
    assert tool.count('if uses == 0 {')==1
    mutant.write_text(tool.replace('if uses == 0 {','if false {'))
    # Keep the mutant outside the fixture's scan directory.
    fixture.unlink()
    scan_dir=directory/'input'
    scan_dir.mkdir()
    (scan_dir/'fixture.go').write_text('package fixture\nfunc unused(arrivedID int) {}\n')
    mutated=subprocess.run(['go','run',str(mutant),str(scan_dir)],cwd=ROOT,env=ENV,text=True,capture_output=True,check=True)
    negative=subprocess.run(['python3','-c',
                             'import json,sys; a=json.load(sys.stdin); assert any(s["value"]=="arrivedID" for s in a["unused_parameters"]), "arrivedID control missed"'],
                            input=mutated.stdout,env=ENV,text=True,capture_output=True)
    check('unused-parameter AST mutant is red on arrivedID',negative.returncode==1,
          dict(exit=negative.returncode,stderr=negative.stderr))

# No missing class, route, reason or missing handler-zero entry is allowed.
for name in ('go-clones','js-clones','sql-statements','sql-production-test','sql-fragments','hex-sql','deadcode','unused-parameters','repeated-literals'):
    data=json.loads(gzip.decompress((HERE/f'classified/{name}.json.gz').read_bytes())) if name=='repeated-literals' else json.loads((HERE/f'classified/{name}.json').read_text())
    check('complete classification '+name,
          all('owner_rows' in g and g['classification'] and g['reason'] and all(1<=r<=19 for r in g['owner_rows']) for g in data),len(data))
db=json.loads((HERE/'classified/handler-db-baseline.json').read_text())
actual=set(str(p.relative_to(ROOT)) for p in (ROOT/'server/api/handlers').glob('*.go') if not p.name.endswith('_test.go'))
check('DB baseline includes every production handler file',actual=={s['file'] for s in db['files']},dict(files=len(actual),total=db['total']))
calls=[s for s in AST['calls'] if s['file'].startswith('server/api/handlers/') and not s['file'].endswith('_test.go') and s['value'] in db['methods']]
unknown=[s for s in calls if s.get('receiver') not in db['receivers'] and s.get('receiver')!='r.URL']
check('DB receiver inventory has no unclassified call',not unknown,dict(excluded_url_query=sum(s.get('receiver')=='r.URL' for s in calls),unknown=unknown))
(HERE/'classified/verification.json').write_text(json.dumps(checks,indent=2,ensure_ascii=False)+'\n')
print(f'{len(checks)} checks passed; source controls hit, planted copy hit then disappeared')
