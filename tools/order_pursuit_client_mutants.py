#!/usr/bin/env python3
"""Run client/API pursuit mutants in a separate clean checkout.

Usage:
  git -C REPO worktree add --detach /tmp/pursuit-mutants SOURCE_COMMIT
  python3 tools/order_pursuit_client_mutants.py /tmp/pursuit-mutants OUTPUT

Each Go invocation creates a fresh migrated DB through tools/gotest.sh on an
independently selected free port. The checkout must contain the API regression
file. No live service is contacted. A mutant only counts if a named assertion
fails, then its exact bytes and SHA are restored. All three suites run green
before mutation and again after restoration.
"""
from pathlib import Path
import argparse
import hashlib
import json
import os
import re
import socket
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('checkout', type=Path)
parser.add_argument('output', type=Path)
args = parser.parse_args()
checkout = args.checkout.resolve()
out = args.output.resolve()
root = Path(__file__).resolve().parents[1]
assert checkout != root, 'use a separate checkout, never the working source tree'
assert not out.is_relative_to(checkout), 'proof output must live outside the mutation checkout'
out.mkdir(parents=True, exist_ok=True)


def git(*arguments):
    return subprocess.check_output(['git', '-C', str(checkout), *arguments], text=True).strip()


assert not git('status', '--porcelain'), 'use a clean checkout'
source = git('rev-parse', 'HEAD')
assert (checkout / 'server/api/handlers/unit_order_pursuit_api_test.go').exists(), 'source must include API regression commit e38aab28 or its descendant'
js = ['node', '--test', 'web/static/js/megaron/ui/runner_status.test.mjs',
      'web/static/js/megaron/ui/stance.test.mjs', 'web/static/js/megaron/ui/marchctx.test.mjs',
      'web/static/js/megaron/ui/war_cards.test.mjs']
keryx = [str(checkout / 'tools/gotest.sh'), './cmd/keryx', '-run', '^TestOrdersEnRoutePursuit', '-v']
api = [str(checkout / 'tools/gotest.sh'), './api/handlers', '-run', '^TestOrderPursuitAPI', '-v']
suites = {'js': js, 'keryx': keryx, 'api': api}
cases = [
    ('keryx-json-field', 'server/cmd/keryx/cmd_unit.go', 'json:"following_unit"', 'json:"follow_unit"', 'keryx', 'TestOrdersEnRoutePursuitUsesNextMeeting'),
    ('keryx-pending', 'server/cmd/keryx/cmd_unit_orders_en_route.go', 'if o.FollowingUnit {', 'if false && o.FollowingUnit {', 'keryx', 'TestOrdersEnRoutePursuitUsesNextMeeting'),
    ('web-runner-status', 'web/static/js/megaron/ui/runner_status.js', 'if (runner.following_unit) {', 'if (false && runner.following_unit) {', 'js', 'order pursuit: next meeting time never promises delivery to a moving unit'),
    ('web-stance-receipt', 'web/static/js/megaron/ui/stance.js', 'if (d?.following_unit) return', 'if (false && d?.following_unit) return', 'js', 'stance receipt: following-unit time is the next meeting, not guaranteed delivery'),
    ('web-march-receipt', 'web/static/js/megaron/ui/marchctx.js', 'dispatched && first.data.following_unit', 'false && dispatched && first.data.following_unit', 'js', 'march receipt: a following Runner'),
    ('web-recall-redirect-receipt', 'web/static/js/megaron/ui/drawers/war.js', 'if (d?.following_unit) return', 'if (false && d?.following_unit) return', 'js', 'recall and redirect receipts qualify the first meeting when the Runner follows'),
    ('api-opt-in', 'server/api/handlers/unit.go', 'payload.FollowUnit = category == string(unit.CategoryLand)', 'payload.FollowUnit = false && category == string(unit.CategoryLand)', 'api', 'TestOrderPursuitAPI_DispatchOptInPersists'),
    ('api-owner-metadata', 'server/api/handlers/world.go', 'if m.Own && orderUnitID != nil {', 'if false && m.Own && orderUnitID != nil {', 'api', 'TestOrderPursuitAPI_MapMetadataOnlyForOwner/owner'),
    # Removing this inner guard alone is observationally redundant: foreign
    # markers pass through foreignCivilianProjection before JSON encoding.
    ('api-foreign-redundant-guard', 'server/api/handlers/world.go', 'if m.Own && orderUnitID != nil {', 'if orderUnitID != nil {', 'api', None),
]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def run(suite, label):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    env = {**os.environ, 'GOTEST_PORT': str(port)}
    result = subprocess.run(suites[suite], cwd=checkout, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (out / (label + '.log')).write_text(result.stdout)
    return result


originals = {rel: (checkout / rel).read_bytes() for _, rel, *_ in cases}
manifest = {'source_commit': source, 'checkout': str(checkout),
            'suites': suites, 'mutants': [], 'baseline_green': [], 'restored_green': []}
(out / 'source.json').write_text(json.dumps({'source_commit': source, 'targets': {rel: sha(raw) for rel, raw in originals.items()}}, indent=2) + '\n')
for suite in suites:
    result = run(suite, 'baseline-' + suite)
    assert result.returncode == 0, (suite, 'baseline failed', result.stdout[-3000:])
    manifest['baseline_green'].append(suite)

try:
    for name, rel, old, new, suite, expected in cases:
        path = checkout / rel
        original = originals[rel]
        assert path.read_bytes() == original, (name, 'source changed before mutation')
        assert original.count(old.encode()) == 1, (name, 'replacement must match exactly once')
        try:
            mutated = original.replace(old.encode(), new.encode(), 1)
            path.write_bytes(mutated)
            result = run(suite, name)
            if expected is None:
                assert result.returncode == 0, (name, 'documented redundant guard unexpectedly changed API observations')
            else:
                assert result.returncode != 0, (name, 'mutant survived')
            assert '[build failed]' not in result.stdout and 'SyntaxError' not in result.stdout, (name, 'not an assertion failure')
            if expected is None:
                pass
            elif suite == 'js':
                assert re.search(r'not ok \d+ - ' + re.escape(expected), result.stdout), (name, 'expected named test did not fail')
                assert 'ERR_ASSERTION' in result.stdout, (name, 'missing JS assertion failure')
            else:
                assert '--- FAIL: ' + expected in result.stdout, (name, 'expected named test did not fail')
            entry = {'name': name, 'file': rel, 'old': old, 'new': new,
                     'expected_test': expected, 'assertion_red': expected is not None,
                     'observationally_redundant_survivor': expected is None,
                     'source_sha256': sha(original), 'mutated_sha256': sha(mutated)}
            print('observationally-redundant-survivor' if expected is None else 'assertion-red', name, flush=True)
        finally:
            path.write_bytes(original)
            assert path.read_bytes() == original and sha(path.read_bytes()) == sha(original)
        entry['restored_sha256'] = sha(path.read_bytes())
        manifest['mutants'].append(entry)
        (out / 'verified.json').write_text(json.dumps(manifest, indent=2) + '\n')
finally:
    for rel, raw in originals.items():
        (checkout / rel).write_bytes(raw)
        assert sha((checkout / rel).read_bytes()) == sha(raw)

assert not git('status', '--porcelain'), 'checkout differs after exact restoration'
for suite in suites:
    result = run(suite, 'restored-' + suite)
    assert result.returncode == 0, (suite, 'restored suite failed', result.stdout[-3000:])
    manifest['restored_green'].append(suite)
manifest['clean_checkout_after_restore'] = not git('status', '--porcelain')
(out / 'verified.json').write_text(json.dumps(manifest, indent=2) + '\n')
print('PASS', sum(case[-1] is not None for case in cases), 'named assertion-red mutants + 1 documented redundant-guard survivor; exact SHA restore; JS/Keryx/API fresh green', flush=True)
