#!/usr/bin/env python3
"""Physical third-party route leak and active-return-window mutations.
Run only while no other Go build/test is reading the shared worktree.
"""
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/fardvag'
OUT.mkdir(parents=True, exist_ok=True)


def mutation(label, path, old, new, test, expected):
    file = ROOT / path
    original = file.read_bytes()
    try:
        source = original.decode()
        assert source.count(old) == 1, (label, source.count(old))
        file.write_text(source.replace(old, new, 1))
        run = subprocess.run(['tools/gotest.sh', './api/handlers/', '-run', test],
                             cwd=ROOT, text=True, capture_output=True)
        output = run.stdout + run.stderr
        (OUT / ('mutation-api-' + label + '.log')).write_text(output)
        assert run.returncode != 0 and expected in output, output
        print(label + ': named red')
    finally:
        file.write_bytes(original)


if '--agreement-only' not in sys.argv:
    mutation('foreign-path', 'server/api/handlers/map_actor_routes.go',
             'if caravan {', 'out["path"] = [][2]int{{999, 999}}\n if caravan {',
             'TestFardvagThirdParty', 'third-party map actor leaked path')
    mutation('return-window', 'server/api/handlers/world.go',
             'legStart, legEnd = *returnDepartsAt, m.ArrivesAt',
             'legStart, legEnd = *returnDepartsAt, returnDepartsAt.Add(m.ArrivesAt.Sub(legStart))',
             'TestFardvagReturningRunnerUsesActualActiveWindow',
             'returning runner current position used obsolete outbound/stay duration')
mutation('agreed-recipient', 'server/api/handlers/world.go',
         'm.RouteKnown = m.Mine || (m.Role == "recipient" && agreedTransportKind(m.Kind))',
         'm.RouteKnown = m.Mine',
         'TestFardvagAgreedRecipientKnowsRouteGiftDoesNot',
         'agreed recipient lost frozen route and costs')
mutation('gift-agreement', 'server/api/handlers/map_actor_routes.go',
         'return kind == "trade" || kind == "trade_return"',
         'return kind == "trade" || kind == "trade_return" || kind == "gift"',
         'TestFardvagAgreedRecipientKnowsRouteGiftDoesNot',
         'unagreed gift recipient received route knowledge')
with (OUT / 'restored-api.log').open('w') as log:
    subprocess.run(['tools/gotest.sh', './api/handlers/', './cmd/keryx/', '-run',
                    'TestFardvag|TestForeignCivilianSightings|TestAcceptance3'],
                   cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=True)
print('API: restored green')
