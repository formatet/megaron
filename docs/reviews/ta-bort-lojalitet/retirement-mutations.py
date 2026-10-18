#!/usr/bin/env python3
"""Physical retirement mutations: named RED, identical restoration, fresh PG GREEN."""
import hashlib
import os
import socket
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[3]
logs = pathlib.Path(__file__).resolve().parent
cases = [
    ('seed', 'server/cmd/server/main.go',
     'tickTypes := []events.ScheduledEventType{',
     'tickTypes := []events.ScheduledEventType{\n\t\tevents.ScheduledLoyaltyDecayTick,',
     './cmd/server/', 'TestSeedDailyTicks_NoLoyalty'),
    ('migration', 'server/db/migrations/185_remove_loyalty.up.sql',
     'AND processed_at IS NULL;', 'AND false;',
     './internal/events/', 'TestRemoveLoyaltyMigrationPreservesOtherStateAndAudit'),
    ('noop', 'server/internal/events/retired_tick.go',
     'func RetiredLoyaltyTick(context.Context, ScheduledEvent) error { return nil }',
     'func RetiredLoyaltyTick(context.Context, ScheduledEvent) error { return context.Canceled }',
     './internal/events/', 'TestRetiredLoyaltyTicksCompleteWithoutEffects'),
]


def run(name, phase, package, test):
    # Ask the kernel for a free local port rather than collide with another
    # agent's parallel fresh-DB rig in gotest.sh's small random port range.
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    environment = dict(os.environ, GOTEST_PORT=str(port))
    result = subprocess.run(
        [str(root / 'tools/gotest.sh'), package, '-run', '^' + test + '$'],
        cwd=root, env=environment, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (logs / ('retirement-mutation-' + name + '-' + phase + '.log')).write_text(result.stdout)
    return result


for name, relative, old, new, package, test in cases:
    path = root / relative
    original = path.read_bytes()
    assert original.count(old.encode()) == 1, (name, 'ambiguous mutation')
    digest = hashlib.sha256(original).hexdigest()
    try:
        path.write_bytes(original.replace(old.encode(), new.encode()))
        result = run(name, 'red', package, test)
        assert result.returncode != 0 and '--- FAIL: ' + test in result.stdout, (name, 'not a named test failure')
        assert 'färsk DB på migration 185' in result.stdout, (name, 'migration setup did not succeed')
    finally:
        path.write_bytes(original)
    assert path.read_bytes() == original, (name, 'restoration mismatch')
    result = run(name, 'restored-green', package, test)
    assert result.returncode == 0 and 'färsk DB på migration 185' in result.stdout, (name, 'restored fresh run failed')
    print(name + ': named RED -> identical bytes restored (sha256 ' + digest + ') -> fresh PG GREEN', flush=True)
