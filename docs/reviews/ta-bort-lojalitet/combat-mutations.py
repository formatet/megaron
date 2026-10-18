#!/usr/bin/env python3
"""Physical combat mutations; every case restores identical bytes before fresh PG GREEN."""
import pathlib, subprocess
root = pathlib.Path(__file__).resolve().parents[3]
logs = pathlib.Path(__file__).resolve().parent
cases = [
    ('participation', 'server/internal/combat/battle.go', 'combatParticipation = 0.90', 'combatParticipation = 0.80', 'TestBattleParticipationFixedAcrossCityAndKharis'),
    ('rout', 'server/internal/combat/resolver.go', 'combatRoutFraction = 0.20', 'combatRoutFraction = 0.50', 'TestFixedRoutThresholdAndHistoricalStandingOrders'),
    ('desertion', 'server/internal/combat/upkeep.go', 'lost := upkeepDesertionStep', 'lost := upkeepDesertionStep * 2', 'TestUnpaidUpkeepHasFixedDesertionStep'),
]
for name, relative, old, new, test in cases:
    path = root / relative
    original = path.read_bytes()
    assert original.count(old.encode()) == 1, (name, 'ambiguous mutation')
    try:
        path.write_bytes(original.replace(old.encode(), new.encode()))
        result = subprocess.run([str(root/'tools/gotest.sh'), './internal/combat', '-run', '^'+test+'$'], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (logs/('combat-mutation-'+name+'-red.log')).write_text(result.stdout)
        assert result.returncode != 0 and '--- FAIL: '+test in result.stdout, (name, 'not a named test failure')
    finally:
        path.write_bytes(original)
    assert path.read_bytes() == original, (name, 'restoration mismatch')
    result = subprocess.run([str(root/'tools/gotest.sh'), './internal/combat', '-run', '^'+test+'$'], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (logs/('combat-mutation-'+name+'-restored-green.log')).write_text(result.stdout)
    assert result.returncode == 0, (name, 'restored run failed')
    print(name+': named RED -> identical bytes restored -> fresh PG GREEN', flush=True)
