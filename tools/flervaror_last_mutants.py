#!/usr/bin/env python3
"""Run four cargo contract mutants on fresh databases, restoring exact source after each."""
from pathlib import Path
import subprocess
root=Path(__file__).resolve().parents[1]
p=root/'server/api/handlers/province.go';d=root/'server/internal/economy/shipment_v2.go'
original=p.read_text(); delivery=d.read_text()
mutants=[('per-good',p,original.replace('if total > ship.Capacity {','if func() bool { for _,it:=range items {if it.Quantity>ship.Capacity{return true}};return false }() {',1)),('old-weight',p,original.replace('if total > ship.Capacity {','if func() bool {w:=0.0;for _,it:=range items {weight,_,_:=economy.IsShippableGood(r.Context(),tx,it.GoodKey);w+=weight*it.Quantity};return w>ship.Capacity}() {',1)),('no-rejection',p,original.replace('if total > ship.Capacity {','if false {',1)),('first-only',d,delivery.replace('for _, it := range items {\n\t\tline :=','for _, it := range items[:1] {\n\t\tline :=',1))]
for name,path,mutated in mutants:
 base=original if path==p else delivery
 if mutated==base:raise RuntimeError(name+' missing pattern')
 try:
  path.write_text(mutated)
  with open('/tmp/flervaror-mutant-'+name+'.log','w') as log:
   result=subprocess.run(['tools/gotest.sh','./api/handlers/','-run','^TestTransferManifestAtomicDelivery$'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
  print(name,result.returncode,flush=True)
  if result.returncode==0:raise RuntimeError(name+' survived')
 finally:path.write_text(base)
