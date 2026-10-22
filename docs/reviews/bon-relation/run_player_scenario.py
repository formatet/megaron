#!/usr/bin/env python3
"""Run browser-dependent acceptance separately from portable Go tests."""
from pathlib import Path
import subprocess
root=Path(__file__).resolve().parents[3]
source=Path(__file__).with_name('player_scenario.go.txt')
target=root/'server/api/handlers/zz_order23_player_scenario_test.go'
assert not target.exists()
try:
 target.write_bytes(source.read_bytes())
 with source.with_name('player-scenario.log').open('w') as log:
  result=subprocess.run(['bash','tools/gotest.sh','./api/handlers','-run','^TestRelationPlayerScenario$','-v'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
 assert result.returncode==0,'See player-scenario.log'
finally:target.unlink(missing_ok=True)
