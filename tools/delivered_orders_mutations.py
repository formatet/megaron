#!/usr/bin/env python3
"""Physical regression controls for delivered course orders; restores each file."""
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
OUT.mkdir(parents=True, exist_ok=True)
CASES = [
    ('moving-course', 'server/internal/combat/march_start.go',
     'redirecting := o.Delivered &&', 'redirecting := false &&',
     ['tools/gotest.sh', './internal/messenger', '-run', 'TestDeliveredMarchReplacesMovingCourse/new_delivered_order'],
     'TestDeliveredMarchReplacesMovingCourse'),
    ('late-runner', 'server/api/handlers/unit.go',
     'interceptPos = target\n\t\tcatchUp = "at_destination"',
     'writeError(w, 422, "no Runner can catch this unit")\n\t\treturn',
     ['tools/gotest.sh', './api/handlers', '-run', 'TestRecall_LateRunnerGoesToDestination'],
     'TestRecall_LateRunnerGoesToDestination'),
    ('escaped-notice', 'web/static/js/megaron/ui/format.js',
     '${esc(body.reason)}` : `${prefix}${what}${target}${delivery}`',
     '${body.reason}` : `${prefix}${what}${target}${delivery}`',
     ['node', '--test', '--test-name-pattern=failed order escapes', 'web/static/js/megaron/ui/format.test.mjs'],
     'failed order escapes text'),
]
for name, relative, old, new, command, expected in CASES:
    path = ROOT / relative
    original = path.read_text()
    assert original.count(old) == 1, (name, 'mutation anchor changed')
    try:
        path.write_text(original.replace(old, new, 1))
        result = subprocess.run(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        log = result.stdout.decode()
        (OUT / (name + '.log')).write_text(log)
        assert result.returncode and expected in log and ('FAIL' in log or 'not ok' in log), (name, log[-3000:])
        print(name + ': named RED', flush=True)
    finally:
        path.write_text(original)
