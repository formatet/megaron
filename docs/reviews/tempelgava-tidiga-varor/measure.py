#!/usr/bin/env python3
"""Read-only temple-offering geography; no world mutations or forced ticks."""
import argparse
import json
import pathlib
import subprocess
import urllib.request

WORLD = 'f7c90afa-4653-46a6-b14c-473a268efa3e'
SQL = """BEGIN READ ONLY;
SELECT json_build_object(
 'world',(SELECT row_to_json(w) FROM (SELECT id,current_tick,state,map_width,map_height FROM worlds WHERE id='WORLD') w),
 'migration',(SELECT row_to_json(m) FROM schema_migrations m),
 'tiles',(SELECT json_agg(t ORDER BY q,r) FROM map_tiles t WHERE world_id='WORLD'),
 'rules',(SELECT json_agg(p ORDER BY id) FROM production_rules p),
 'goods',(SELECT json_agg(g ORDER BY key) FROM (SELECT key,labor_value,status FROM goods) g),
 'occupied',(SELECT COALESCE(json_agg(o),'[]') FROM (
   SELECT map_q AS q,map_r AS r FROM provinces WHERE world_id='WORLD'
   UNION ALL SELECT u.q,u.r FROM units u JOIN founder_phase f ON f.host_unit_id=u.id AND f.active
   WHERE u.world_id='WORLD' AND u.q IS NOT NULL AND u.r IS NOT NULL) o)
); COMMIT;""".replace('WORLD', WORLD)
EXCLUDED = {'coastal_sea', 'deep_sea', 'river', 'river_ford', 'mountain_limestone', 'mountain_red', 'semi_desert'}


def distance(a, b):
    dq, dr = a['q'] - b['q'], a['r'] - b['r']
    return max(abs(dq), abs(dr), abs(dq + dr))


def analyse(snapshot):
    tiles = snapshot['tiles']
    by_coord = {(t['q'], t['r']): t for t in tiles}
    rules = snapshot['rules']
    active_goods = {g['key'] for g in snapshot['goods'] if g['status'] == 'active'}
    timber_terrains = {r['terrain_type'] for r in rules if r['good_key'] == 'timber' and r['building_type'] is None and r['terrain_type'] is not None}
    viable = {t['landmass_id'] for t in tiles if t['landmass_id'] is not None and t['terrain'] in timber_terrains}
    rows = []
    for t in tiles:
        if t['terrain'] in EXCLUDED or (t['landmass_id'] is not None and t['landmass_id'] not in viable):
            continue
        if any(distance(t, o) <= 4 for o in snapshot['occupied']):
            continue
        ring = [by_coord[(t['q'] + dq, t['r'] + dr)] for dq in range(-2, 3) for dr in range(-2, 3)
                if 0 < max(abs(dq), abs(dr), abs(dq + dr)) <= 2 and (t['q'] + dq, t['r'] + dr) in by_coord]
        best = dict.fromkeys(('grain', 'fish', 'timber', 'silver'), 0.0)
        for n in ring:
            for rule in rules:
                good = rule['good_key']
                if good not in best or good not in active_goods:
                    continue
                if rule['terrain_type'] is not None and rule['terrain_type'] != n['terrain']:
                    continue
                if rule['requires_coastal'] and not n['coastal']:
                    continue
                if rule['requires_deposit'] and not n[rule['requires_deposit'] + '_deposit']:
                    continue
                # No early buildings; silver is measured with a level-1 mine.
                if rule['building_type'] is not None and not (good == 'silver' and rule['building_type'] == 'mine'):
                    continue
                if n['terrain'] in {'coastal_sea', 'deep_sea', 'river', 'river_ford'} and rule['terrain_type'] != n['terrain']:
                    continue
                best[good] = max(best[good], rule['rate_per_tick'] * (1.7 if good == 'silver' else 1))
        rows.append({'q': t['q'], 'r': t['r'], 'terrain': t['terrain'], 'coast': any(n['terrain'] in {'coastal_sea', 'deep_sea'} for n in ring),
                     'forest': any(n['terrain'] in timber_terrains for n in ring),
                     'silver_deposit': any(n['silver_deposit'] for n in ring), 'best_per_worker': best})
    assert rows, 'No eligible spawn candidates'
    n = len(rows)
    counts = {key: sum(r[key] for r in rows) for key in ('forest', 'coast', 'silver_deposit')}
    counts.update({g: sum(r['best_per_worker'][g] > 0 for r in rows) for g in ('grain', 'fish', 'timber', 'silver')})
    counts['grain_or_fish'] = sum(r['best_per_worker']['grain'] > 0 or r['best_per_worker']['fish'] > 0 for r in rows)
    counts['any_early_good'] = sum(any(r['best_per_worker'].values()) for r in rows)
    counts['grain_and_timber'] = sum(r['best_per_worker']['grain'] > 0 and r['best_per_worker']['timber'] > 0 for r in rows)
    return {'population': 'All eligible join terrain/viable-landmass/clearance candidates before load/ore ranking; worked radius 2 excludes city centre; no FOW, siege or ownership filter',
            'candidates': n, 'counts': counts, 'percent': {k: round(100 * v / n, 2) for k, v in counts.items()},
            'rates_per_worker': {g: sorted({round(r['best_per_worker'][g], 8) for r in rows}) for g in ('grain', 'fish', 'timber', 'silver')},
            'uncovered_examples': [r for r in rows if not any(r['best_per_worker'].values())][:10], 'candidate_rows': rows}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--snapshot', type=pathlib.Path, help='Replay an already captured snapshot without SSH')
    args = parser.parse_args()
    here = pathlib.Path(__file__).resolve().parent
    if args.snapshot:
        captured = json.loads(args.snapshot.read_text())
    else:
        result = subprocess.run(['ssh', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=8', 'root@10.0.1.92',
                                 'runuser -u postgres -- psql -d poleia -X -qAt -v ON_ERROR_STOP=1'],
                                input=SQL, text=True, capture_output=True, check=True)
        captured = {'health': json.load(urllib.request.urlopen('http://10.0.1.92:8080/healthz', timeout=8)),
                    'snapshot': json.loads(result.stdout)}
        (here / 'snapshot.json').write_text(json.dumps(captured, indent=2) + '\n')
    measured = analyse(captured['snapshot'])
    (here / 'measurement.json').write_text(json.dumps(measured, indent=2) + '\n')
    print(json.dumps({k: v for k, v in measured.items() if k != 'candidate_rows'}, indent=2))


if __name__ == '__main__':
    main()
