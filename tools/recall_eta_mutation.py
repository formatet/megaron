#!/usr/bin/env python3
"""Prove the retained live recall ETA regression catches wall-hour travel."""
from pathlib import Path
import os
import re
import socket
import subprocess
import sys
ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
OUT.mkdir(parents=True, exist_ok=True)
def run(name, red=False):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    result = subprocess.run(['tools/gotest.sh', './internal/messenger', '-run', 'TestSingleRecall_ArrivesAtMatchesTickSchedule'], cwd=ROOT,
        env={**os.environ, 'GOTEST_PORT': str(port)}, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (OUT/(name+'.log')).write_text(result.stdout)
    if red:
        assert result.returncode and re.search(r'--- FAIL: TestSingleRecall_ArrivesAtMatchesTickSchedule',result.stdout) and 'wall-hour ETA' in result.stdout and 'build failed' not in result.stdout, result.stdout
    else:
        assert result.returncode == 0, result.stdout
    print(name + (': RED (ETA assertion)' if red else ': GREEN'), flush=True)
run('baseline')
path = ROOT/'server/internal/combat/recall_redirect.go'
original = path.read_text()
old = 'now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)'
assert old in original
try:
    path.write_text(original.replace(old, 'now.Add(time.Duration(moveTicks * float64(time.Hour)))', 1))
    run('wall-hours', True)
finally:
    path.write_text(original)
run('restored')
