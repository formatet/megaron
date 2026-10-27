from pathlib import Path
import subprocess,json,sys,socket,os
r=Path(sys.argv[1]).resolve() if len(sys.argv)>1 else Path(__file__).resolve().parents[1];out=Path(sys.argv[2]).resolve() if len(sys.argv)>2 else r/'docs/reviews/kult-flikar/mutations-server';out.mkdir(parents=True,exist_ok=True)
def run(cmd):
 env=os.environ.copy()
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
 return subprocess.run(cmd,cwd=r,capture_output=True,text=True,env=env)
cases=[
 ('cult.go','AND owner_id=$2 AND state','AND owner_id IS NOT NULL AND $2::uuid IS NOT NULL AND state'),
 ('cult.go','if p.Affordable {','if false && p.Affordable {'),
 ('cult_prayers.go','canPrepareOffering := afford','canPrepareOffering := false'),
 ('province.go','worldID, kharisNow, goodsStock)','worldID, 0, goodsStock)'),
]
proof=[]
for i,(f,old,new) in enumerate(cases):
 p=r/'server/api/handlers'/f;before=p.read_bytes();assert before.count(old.encode())==1,(f,old)
 try:
  p.write_bytes(before.replace(old.encode(),new.encode(),1))
  res=run([str(r/'tools/gotest.sh'),'./api/handlers/','-run','^TestRealmCultThreeTemplesTwoGodsAndOwnerPrivacy$'])
  log=res.stdout+res.stderr;(out/f'{i}-{f}-red.log').write_text(log)
  assert res.returncode!=0 and '--- FAIL: TestRealmCultThreeTemplesTwoGodsAndOwnerPrivacy' in log and '[build failed]' not in log,(f,log)
  proof.append({'file':str(p.relative_to(r)),'old':old,'new':new,'named_red':'TestRealmCultThreeTemplesTwoGodsAndOwnerPrivacy','assertion_red':True})
 finally:p.write_bytes(before)
 assert p.read_bytes()==before
 print('RED + byte-identical restore',f,flush=True)
res=run([str(r/'tools/gotest.sh'),'./api/handlers/','-run','TestRealmCultThreeTemples|TestAvailablePrayers|TestTemplePerGod_Prayer|TestRite.*'])
(out/'restored-green.log').write_text(res.stdout+res.stderr);assert res.returncode==0,res.stdout+res.stderr
(out/'proof.json').write_text(json.dumps({'mutants':proof,'byte_identical_restore':True,'fresh_restored_green':True},indent=2))
print('STABLE restored fresh green',flush=True)
