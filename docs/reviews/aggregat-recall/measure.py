#!/usr/bin/env python3
"""Measure canonical saved-route positions for equal target and arrival tick."""
import os
from pathlib import Path
import subprocess
root=Path(__file__).resolve().parents[3]
test=root/"server/internal/combat/aggregat_recall_premise_test.go"
assert not test.exists()
try:
 test.write_text('''package combat
import ("testing"; "formatet/megaron/server/internal/province")
func TestAggregateRecallPremise(t *testing.T){
 routes:=[][]province.MapPosition{
  {{Q:0,R:0},{Q:1,R:0},{Q:2,R:0},{Q:3,R:0},{Q:4,R:0}},
  {{Q:4,R:-4},{Q:4,R:-3},{Q:4,R:-2},{Q:4,R:-1},{Q:4,R:0}},
 }
 var positions []province.MapPosition
 for i,path:=range routes{
  route,ok:=BuildRoute(path,[]float64{1,1,1,1},0,10);if !ok{t.Fatal("invalid fixture route")}
  pos,err:=RoutePositionAt(route,5000);if err!=nil{t.Fatal(err)}
  positions=append(positions,pos)
  t.Logf("unit %d: target=(4,0), depart_tick=0, arrive_tick=10; tick 5 position=(%d,%d)",i+1,pos.Q,pos.R)
 }
 if positions[0]==positions[1]{t.Fatal("fixture did not demonstrate different places")}
}
''')
 result=subprocess.run(["go","test","./internal/combat","-run","TestAggregateRecallPremise","-v","-count=1"],cwd=root/"server",env={k:os.environ[k] for k in ("HOME","PATH")},capture_output=True,text=True)
 print(result.stdout+result.stderr)
 assert result.returncode==0
finally:
 test.unlink(missing_ok=True)
