#!/usr/bin/env python3
"""Read-only actual-helper proof: temporary test harness, always removed.
No DB, production-file edits or game writes. Run from repo root.
"""
from pathlib import Path
import os,subprocess
ROOT=Path(__file__).resolve().parents[3]
p=ROOT/'server/internal/kharis/slice19_cap_probe_test.go'
assert not p.exists()
source='''package kharis
import("testing";"fmt")
func TestSlice19CapProbe(t *testing.T){
 cap:=EffectiveKharisCap(100,0)
 actual:=clampKharis(40-0.1,cap)
 fmt.Printf("ACTUAL_BASELINE cap0=%.1f proposed_start40_after_first_decay=%.1f\\n",cap,actual)
 if cap!=25 || actual!=25 {t.Fatal("baseline changed; reassess the simulation")}
}
'''
try:
 p.write_text(source)
 env={k:os.environ[k] for k in ('HOME','PATH')}
 r=subprocess.run(['go','test','./internal/kharis','-run','^TestSlice19CapProbe$','-count=1','-v'],cwd=ROOT/'server',env=env,capture_output=True,text=True)
 (Path(__file__).parent/'cap-probe.log').write_text(r.stdout+r.stderr)
 assert r.returncode==0,r.stdout+r.stderr
 print(r.stdout)
finally:p.unlink(missing_ok=True)
