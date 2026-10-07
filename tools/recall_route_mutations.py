#!/usr/bin/env python3
"""Fresh-DB assertion mutations for recall route/position and visible refusal."""
from pathlib import Path
import os
import socket
import re
import subprocess
import sys
ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
OUT.mkdir(parents=True, exist_ok=True)
def run(name, package, pattern, red=False):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    p = subprocess.run(['tools/gotest.sh', package, '-run', pattern], cwd=ROOT,
                       env={**os.environ, 'GOTEST_PORT': str(port)},
                       text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (OUT/(name+'.log')).write_text(p.stdout)
    failures = re.findall(r'--- FAIL: (\S+)', p.stdout)
    if red:
        assert p.returncode and any(re.search(pattern, f) for f in failures) and 'build failed' not in p.stdout, (name, p.stdout)
    else:
        assert p.returncode == 0, (name, p.stdout)
    print(name + (': RED '+','.join(failures) if red else ': GREEN'), flush=True)
run('baseline-messenger', './internal/messenger', 'TestRecallNoRoute')
run('baseline-http', './api/handlers', 'TestRecallNoRoute')
mutations = [
 ('course-guard', 'server/internal/combat/recall_redirect.go', 'if !pathOK {', 'if false && !pathOK {', './internal/messenger', 'TestRecallNoRoute_CoreRejectsWithoutMutation'),
 ('position-guard', 'server/internal/combat/recall_redirect.go', 'if !posOK {\n\t\t\treturn nil, reject', 'if false && !posOK {\n\t\t\treturn nil, reject', './internal/messenger', 'TestRecallNoRoute_CoreRejectsWithoutMutation'),
 ('delivery-reason', 'server/internal/messenger/order_delivery.go', 'return h.failSingleRecall(ctx, p, rej.Reason)', 'return h.failSingleRecall(ctx, p, "order failed")', './internal/messenger', 'TestRecallNoRoute_DeliveryNamesReasonOnce'),
 ('http-reason', 'server/api/handlers/unit.go', 'writeError(w, rej.Status, rej.Reason)', 'writeError(w, 500, "order failed")', './api/handlers', 'TestRecallNoRoute_HTTPRejectsNamed'),
 ('http-position', 'server/api/handlers/unit.go', '"cannot resolve the unit\'s current position: no passable outbound route; check this unit and reissue the order"', '"order failed"', './api/handlers', 'TestRecallNoRoute_HTTPDoesNotDispatchFromGuessedOrigin'),
]
for name, file, old, new, package, pattern in mutations:
    path = ROOT/file; original = path.read_text()
    assert old in original, name
    # HTTP OrderReject has other verbs; replace only inside Recall's body.
    start = 0
    if name == 'http-reason':
        start = original.index('res, err := combat.ExecuteRecall(')
    elif name == 'delivery-reason':
        start = original.index('case "recall", "redirect":')
    try:
        path.write_text(original[:start]+original[start:].replace(old,new,1))
        subprocess.run(['gofmt','-w',str(path)],check=True)
        run(name, package, pattern, True)
    finally:
        path.write_text(original)
    run(name+'-restored', package, pattern)
