#!/usr/bin/env python3
"""Real-source mutations against named tests, each isolated fresh database."""
import hashlib,json,pathlib,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
cases=[
 ('spear_salary','server/internal/combat/upkeep.go','"spearman":       {Grain: 0.50, Silver: 0}','"spearman":       {Grain: 0.50, Silver: 1}','./internal/combat','^TestSpearmanZeroSilverStillEatsAndNeverDeserts$','ration-only spearman'),
 ('elite_salary','server/internal/combat/upkeep.go','"elite_infantry": {Grain: 0.60, Silver: 2}','"elite_infantry": {Grain: 0.60, Silver: 0}','./internal/combat','^TestSpearmanFieldRationAndElitePayRemain$','elite pay leaves'),
 ('colony_derived','server/internal/combat/march_start.go','return full * float64(unitSize) / 100','return UnitUpkeep(unitType, string(unit.CategoryOf(unit.Type(unitType))), unitSize, "garrison").Silver*30 + 0*full','./internal/combat','^TestColonistPurse_FixedReservesDoNotFollowUpkeep$','reserve=0'),
 ('rise_derived','server/api/handlers/rise.go','silver:     risenHostStartingSilver,','silver:     60 * combat.UnitUpkeep("spearman", "land", 100, "positioned").Silver,','./api/handlers','^TestRise_RefugeeHostOnAnotherLandmass_AndLeave$','want fixed reserve 60'),
 ('newcomer_derived','server/api/handlers/nomadic_host.go','silverAmount := spec.silver','silverAmount := 240 * float64(spec.spearmen) * combat.UnitUpkeep("spearman", "land", 100, "positioned").Silver','./api/handlers','^TestSeedNomadicHost_SilverAmountIs480$','want fixed480'),
]
results=[]
for name,file,before,after,pkg,test,marker in cases:
 p=ROOT/file;original=p.read_bytes();source=original.decode();assert source.count(before)==1,(name,before)
 changed=source.replace(before,after)
 if name in ['rise_derived','newcomer_derived']:
  changed=changed.replace('import (','import (\n "formatet/megaron/server/internal/combat"',1)
 try:
  p.write_text(changed)
  r=subprocess.run([str(ROOT/'tools/gotest.sh'),pkg,'-run',test],cwd=ROOT,text=True,capture_output=True)
  (HERE/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and marker in r.stdout+r.stderr and '[build failed]' not in r.stdout+r.stderr,(name,r.stdout[-1000:],r.stderr[-1000:])
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'mutation':name,'test':test,'red':True,'byte_restored':True,'sha256':hashlib.sha256(original).hexdigest()})
 print('RED -> BYTE RESTORED',name,flush=True)
r=subprocess.run([str(ROOT/'tools/gotest.sh'),'./internal/combat','./api/handlers','-run','TestSpearman|TestColonistPurse_Fixed|TestRise_Refugee|TestSeedNomadicHost_Silver|TestUnitRosterRations'],cwd=ROOT,text=True,capture_output=True)
(HERE/'mutations-restored-green.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(r.stdout,r.stderr)
(HERE/'mutations.json').write_text(json.dumps({'mutations':results,'restored_fresh_green':True},indent=2)+'\n')
print('FRESH GREEN all named tests',flush=True)
