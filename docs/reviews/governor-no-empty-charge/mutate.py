#!/usr/bin/env python3
"""Restore the baseline governor, require named RED, restore exact bytes, require GREEN."""
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[3]
source = root / "server/internal/ai/governor.go"
fixed = source.read_bytes()
original = subprocess.check_output(
    ["git", "-C", str(root), "show", "a7b8c858:server/internal/ai/governor.go"]
)
env = dict(os.environ, GOCACHE="/tmp/megaron-governor-gocache")
command = [str(root / "tools/gotest.sh"), "./internal/ai/", "-run",
           "TestPassiveGovernorTickPreservesStockWithoutAction", "-v"]
try:
    source.write_bytes(original)
    red = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True)
    (Path(__file__).parent / "mutation.log").write_text(red.stdout + red.stderr)
    assert red.returncode != 0 and "pass 1 changed stock/loyalty/events" in red.stdout, red.stdout + red.stderr
finally:
    source.write_bytes(fixed)
assert source.read_bytes() == fixed
green = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True)
(Path(__file__).parent / "restored-green.log").write_text(green.stdout + green.stderr)
assert green.returncode == 0 and "--- PASS: TestPassiveGovernorTickPreservesStockWithoutAction" in green.stdout, green.stdout + green.stderr
print("Named RED → identical source bytes → fresh DB GREEN")
