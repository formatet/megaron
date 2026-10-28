#!/usr/bin/env python3
"""Eight physical Forces server/CLI mutations in a clean isolated checkout.

Usage: python3 tools/forces_server_mutations_20261027.py CHECKOUT OUTPUT
Each run selects an unused TCP port and invokes the fresh-DB gotest harness.
The runner requires named assertion failures, restores exact original bytes,
then reruns all named tests green against another fresh database. No live data.
"""
from pathlib import Path
import hashlib
import json
import os
import socket
import subprocess
import sys

CHECKOUT = Path(sys.argv[1]).resolve()
OUT = Path(sys.argv[2]).resolve()
OUT.mkdir(parents=True, exist_ok=True)

def git(*args):
    return subprocess.check_output(['git', '-C', str(CHECKOUT), *args], text=True).strip()

assert not git('status', '--porcelain'), 'mutation checkout must start clean'
commit = git('rev-parse', 'HEAD')
cases = [
    ('display-label', 'server/internal/unit/model.go', '"Bronze Guard"', '"Elite Infantry"', './internal/unit/', 'TestForcesDisplayTaxonomyAndDynamicLandNames'),
    ('binding-owner', 'server/api/handlers/unit_bound_routes.go', 'AND so.owner_id=$2', 'AND $2::uuid IS NOT NULL', './api/handlers/', 'TestAttachFreightingNotes_PrefersInFlightLegOverStandingRoute'),
    ('trade-recipient-privacy', 'server/api/handlers/world.go', 'if authenticated && ownerID == playerID {\n\t\t\tm.StandingOrderID', 'if authenticated {\n\t\t\tm.StandingOrderID', './api/handlers/', 'TestMapTradesStandingOrderIdentityIsSenderPrivate'),
    ('roster-binding-wire', 'server/api/handlers/unit.go', 'if err := attachBoundRoutes(r.Context(), h.pool, worldID, playerID, summaries); err != nil {', 'if err := error(nil); err != nil {', './api/handlers/', 'TestListForcesIncludesBoundRouteOnIdleShip'),
    ('legacy-recruit-alias', 'server/cmd/keryx/cmd_recruit.go', '"elite infantry": "elite_infantry"', '"elite infantry": "spearman"', './cmd/keryx/', 'TestForcesRecruitLegacyAndCanonicalLabelsAccepted'),
    ('canonical-forces-command', 'server/cmd/keryx/cmd_aliases.go', 'c.Use = "forces"', 'c.Use = "units"', './cmd/keryx/', 'TestForcesRosterKeepsLegacyUnitsAlias'),
    ('bound-route-display', 'server/cmd/keryx/cmd_unit.go', 'fmt.Println("  on the " + *u.BoundRouteName + " route")', 'fmt.Print("")', './cmd/keryx/', 'TestForcesRosterDisplaysBoundRouteAndCurrentLeg'),
    ('people-group', 'server/cmd/keryx/cmd_unit.go', 'if group == "People" && !hasHost {', 'if group == "People" && hasHost {', './cmd/keryx/', 'TestForcesRosterGroupsAndPeopleOnlyWithHost'),
]
files = {f: (CHECKOUT/f).read_bytes() for _, f, *_ in cases}
def hashes():
    return {f: hashlib.sha256((CHECKOUT/f).read_bytes()).hexdigest() for f in files}
source_before = hashes()

def run_fresh(args):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    # No running-server environment is inherited; gotest.sh also cleans Go's env.
    env = {k: os.environ[k] for k in ('HOME', 'PATH', 'GOPATH', 'GOCACHE') if k in os.environ}
    env['GOTEST_PORT'] = str(port)
    result = subprocess.run([str(CHECKOUT/'tools/gotest.sh'), *args], cwd=CHECKOUT,
                            env=env, capture_output=True, text=True)
    return result, port

proof = []
try:
    for name, file, old, new, package, test in cases:
        path = CHECKOUT/file
        before = files[file]
        assert before.count(old.encode()) == 1, (file, old)
        try:
            path.write_bytes(before.replace(old.encode(), new.encode(), 1))
            result, port = run_fresh([package, '-run', '^'+test+'$'])
            log = result.stdout+result.stderr
            (OUT/(name+'-red.log')).write_text(log)
            assert result.returncode != 0 and '--- FAIL: '+test in log, (name, log)
            assert '[build failed]' not in log and 'panic:' not in log and 'fatal error:' not in log, (name, log)
            proof.append({'name': name, 'file': file, 'old': old, 'new': new,
                          'package': package, 'named_test': test, 'assertion_red': True,
                          'port': port, 'source_sha256': source_before[file]})
        finally:
            path.write_bytes(before)
        assert hashes() == source_before, 'source restoration was not byte-identical'
        print('RED + exact restore: '+name, flush=True)
finally:
    for file, content in files.items():
        (CHECKOUT/file).write_bytes(content)

assert hashes() == source_before and not git('status', '--porcelain')
names = sorted({c[-1] for c in cases})
result, port = run_fresh(['./internal/unit/', './cmd/keryx/', './api/handlers/',
                         '-run', '^('+'|'.join(names)+')$'])
(OUT/'restored-green.log').write_text(result.stdout+result.stderr)
assert result.returncode == 0, result.stdout+result.stderr
assert hashes() == source_before and not git('status', '--porcelain')
(OUT/'proof.json').write_text(json.dumps({'commit': commit, 'mutations': proof,
    'source_sha256': source_before, 'byte_identical_restore': True,
    'fresh_restored_green': True, 'green_port': port}, indent=2)+'\n')
print('STABLE: eight assertion-red mutants, exact restore, fresh restored green', flush=True)
