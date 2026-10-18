import pathlib,subprocess
logs=pathlib.Path(__file__).resolve().parent
root=pathlib.Path(__file__).resolve().parents[3]
p=root/'server/internal/combat/upkeep.go'
original=p.read_bytes()
old=b'u.status == "garrison" && u.settlementID != nil && u.category == "land"'
start=original.index(b'\t// City-level starvation transitions')
end=original.index(b'\tlevel := 3',start)
cases=[('elite-witness',original[:original.index(b'func (h *UpkeepHandler) applyAttrition')]+original[original.index(b'func (h *UpkeepHandler) applyAttrition'):].replace(old,b'u.status == "garrison" && u.settlementID != nil && (u.unitType == "spearman" || u.unitType == "war_chariot")')),('routine-notice',original[:start]+original[end:])]
for name,mutated in cases:
 try:
  assert mutated != original
  p.write_bytes(mutated)
  r=subprocess.run([str(root/'tools/gotest.sh'),'./internal/combat','-run','^TestGarrisonAttritionAuditAndNoticeOwnership$'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (logs/('slice14-combat-'+name+'-red.log')).write_text(r.stdout)
  assert r.returncode!=0 and '--- FAIL: TestGarrisonAttritionAuditAndNoticeOwnership' in r.stdout,(name,r.stdout)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 r=subprocess.run([str(root/'tools/gotest.sh'),'./internal/combat','-run','^TestGarrisonAttritionAuditAndNoticeOwnership$'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (logs/('slice14-combat-'+name+'-restored-green.log')).write_text(r.stdout)
 assert r.returncode==0,(name,r.stdout)
 print(name+': named RED -> byte-exact restoration -> fresh PG GREEN',flush=True)
