#!/usr/bin/env python3
"""Run offer-deadline physical mutants in isolated CHECKOUT, writing OUTPUT."""
import hashlib,json,os,pathlib,socket,subprocess,sys
root,out=map(pathlib.Path,sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
cases=[
 ('lifetime-168','server/internal/messenger/handler.go','OfferExpiryTicks = 24 //','OfferExpiryTicks = 168 //',['./internal/messenger','./api/handlers'],'TestNew.*OfferDeadline.*','TestNewSeaOfferDeadlineIsResolvedArrivalPlus24','arrival+24'),
 ('arrival-ignores-saved-expiry','server/internal/messenger/handler.go','if deadline != nil {','if false && deadline != nil {',['./internal/messenger'],'TestExistingOfferArrivalKeepsPersisted168Deadline','TestExistingOfferArrivalKeepsPersisted168Deadline','must retain persisted deadline'),
 ('passage-ignores-saved-expiry','server/internal/messenger/passage.go','if deadline != nil {','if false && deadline != nil {',['./internal/messenger'],'TestExistingSeaOfferReplanKeepsScheduled168Deadline','TestExistingSeaOfferReplanKeepsScheduled168Deadline','replan changed existing168 expiry'),
]
def run(name,pkgs,test):
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 env=os.environ.copy();env['GOTEST_PORT']=str(port)
 p=subprocess.run(['tools/gotest.sh',*pkgs,'-run','^'+test+'$'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (out/(name+'.log')).write_text(p.stdout);return p
summary=[]
for name,file,old,new,pkgs,pattern,test,message in cases:
 path=root/file;original=path.read_bytes();data=original.decode();assert data.count(old)==1
 try:path.write_text(data.replace(old,new,1));red=run(name+'-red',pkgs,pattern)
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
 assert red.returncode!=0 and ('--- FAIL: '+test) in red.stdout and message in red.stdout and 'build failed' not in red.stdout and 'panic:' not in red.stdout,(name,red.stdout[-2200:])
 green=run(name+'-restored-green',pkgs,pattern);assert green.returncode==0,(name,green.stdout[-2200:])
 summary.append({'mutant':name,'file':file,'assertion':test,'red_message':message,'assertion_red':True,'exact_restored_sha256':hashlib.sha256(original).hexdigest(),'fresh_restore_green':True})
 (out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(name+': assertion RED / exact restore / fresh GREEN',flush=True)
