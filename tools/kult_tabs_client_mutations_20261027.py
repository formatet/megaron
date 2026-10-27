#!/usr/bin/env python3
"""Run client mutations in an isolated checkout; require named assertion failures."""
from pathlib import Path
import subprocess,sys,json,socket,os
root=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
cases=[
 ('web/static/js/megaron/ui/drawers/kult_realm.js',"p.settlement_id + '__' + p.id","p.id",['node','--test','web/static/js/megaron/ui/drawers/kult_realm.test.mjs'],'three temples and two gods'),
 ('web/static/js/megaron/ui/drawers/kult.js',"prayer.settlement_id + '/rite'","'wrong-city' + '/rite'",['node','--test','web/static/js/megaron/ui/drawers/kult.test.mjs'],'same god in two cities'),
 ('web/static/js/megaron/ui/drawers/kult_kharis.js','Temple workers across your realm','Temple workers here',['node','--test','web/static/js/megaron/ui/drawers/kult_realm.test.mjs'],'realm workforce is labelled'),
 ('server/cmd/keryx/cmd_cult.go','p.SettlementID, p.ID','p.ProvinceID, p.ID',[str(root/'tools/gotest.sh'),'./cmd/keryx/','-run','TestRealmCultCLIThreeTemplesAndCityPrayerRouting'],'TestRealmCultCLIThreeTemplesAndCityPrayerRouting'),
 ('server/cmd/keryx/cmd_status.go','realmCultShown := printRealmTemples(c)','realmCultShown := false',[str(root/'tools/gotest.sh'),'./cmd/keryx/','-run','TestRealmCultCLIThreeTemplesAndCityPrayerRouting'],'TestRealmCultCLIThreeTemplesAndCityPrayerRouting'),
 ('server/cmd/keryx/cmd_rite.go','return listRealmPrayers(c, settlementID)','return nil',[str(root/'tools/gotest.sh'),'./cmd/keryx/','-run','TestRiteListReadsTheCastsCity'],'TestRiteListReadsTheCastsCity'),
]
def run(cmd):
 env=os.environ.copy()
 if any('gotest.sh' in x for x in cmd):
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
 return subprocess.run(cmd,cwd=root,text=True,capture_output=True,env=env)
proof=[]
for i,(file,old,new,cmd,test) in enumerate(cases):
 p=root/file;before=p.read_bytes();assert old in before.decode(),file
 try:
  p.write_text(before.decode().replace(old,new,1));res=run(cmd);log=res.stdout+res.stderr;(out/f'{i}-red.log').write_text(log)
  assert res.returncode and test in log and ('not ok' in log or '--- FAIL:' in log) and '[build failed]' not in log and 'SyntaxError' not in log,(file,log)
  proof.append({'file':file,'old':old,'new':new,'test':test,'assertion_red':True})
 finally:p.write_bytes(before)
 assert p.read_bytes()==before;print('RED + exact restore',file,flush=True)
for name,cmd in [('js',['node','--test','web/static/js/megaron/ui/drawers/kult.test.mjs','web/static/js/megaron/ui/drawers/kult_realm.test.mjs']),('cli',[str(root/'tools/gotest.sh'),'./cmd/keryx/'])]:
 res=run(cmd);(out/f'restored-{name}-green.log').write_text(res.stdout+res.stderr);assert res.returncode==0,res.stdout+res.stderr
(out/'proof.json').write_text(json.dumps({'mutants':proof,'exact_restore':True,'restored_green':True},indent=2));print('PASS client mutants/restored green',flush=True)
