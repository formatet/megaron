#!/usr/bin/env python3
"""Send explicitly marked Umami test events, then verify using SELECT only.

HTTP writes require --send. Never writes/deletes rows directly. Uses the ordinary
Firefox UA required by Umami's bot filter. See vault umami.md (CT105 on macxpox).
"""
import argparse
import json
from pathlib import Path
import re
import shlex
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
HISTORICAL = {'build_started', 'recruit_started', 'march_sent', 'messenger_sent',
              'trade_offer', 'rite_performed', 'settle', 'livestock_slaughtered'}
UA = 'Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0'


def sql(query):
    psql = shlex.join(['psql', '-X', '-v', 'ON_ERROR_STOP=1', '-d', 'umami', '-Atc', query])
    remote = shlex.join(['pct', 'exec', '105', '--', 'su', 'postgres', '-c', psql])
    return subprocess.check_output(['ssh', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10',
                                    'root@10.0.1.79', remote], text=True, timeout=30).strip()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--send', action='store_true', help='send test:1 HTTP events (explicit authorization required)')
    p.add_argument('--output', type=Path, required=True)
    args = p.parse_args()
    catalog = json.loads(subprocess.check_output([
        'node', '--input-type=module', '-e',
        "import {VERB_EVENTS} from './web/static/js/megaron/verb_telemetry.js'; console.log(JSON.stringify(VERB_EVENTS));"
    ], cwd=ROOT, text=True))
    names = sorted({entry['event'] for entry in catalog} - HISTORICAL | {'verb_refused'})
    assert all(re.fullmatch('[a-z_]+', name) for name in names)
    website = re.search(r'data-website-id="([0-9a-f-]+)"', (ROOT / 'web/static/map.html').read_text())[1]
    start = sql("SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS.US')")
    if not args.send:
        print(json.dumps({'new_events': names, 'send': False, 'db_time_utc': start}, indent=2))
        return
    statuses = {}
    with tempfile.TemporaryDirectory(prefix='umami-verb-response-') as folder:
        for name in names:
            data = {'test': 1}
            if name == 'verb_refused':
                data.update(verb='build', reason_code='insufficient_goods')
            payload = {'type': 'event', 'payload': {
                'website': website, 'hostname': 'megaron.formatet.se', 'screen': '1280x900',
                'language': 'en-US', 'url': '/umami-verb-test', 'name': name, 'data': data,
            }}
            # API responses may contain anonymous session IDs; do not include them
            # in the proof. Only HTTP status and aggregate DB counts are saved.
            code = subprocess.check_output([
                'curl', '--silent', '--show-error', '--max-time', '20', '-A', UA,
                '-H', 'Content-Type: application/json', '--data-binary', '@-',
                '--output', str(Path(folder) / 'response.json'), '--write-out', '%{http_code}',
                'https://umami.formatet.se/api/send',
            ], input=json.dumps(payload), text=True, timeout=25)
            statuses[name] = int(code)
            assert 200 <= int(code) < 300, (name, code)
            print(f'{name}: HTTP {code}', flush=True)
    raw = sql(f"""SELECT w.event_name,count(*) FROM website_event w
        JOIN event_data d ON d.website_event_id=w.event_id
        WHERE w.website_id='{website}' AND w.created_at>='{start}'::timestamp
        AND w.url_path='/umami-verb-test' AND w.event_type=2
        AND d.data_key='test' AND d.number_value=1
        GROUP BY w.event_name ORDER BY w.event_name""")
    counts = {name: int(count) for name, count in (line.split('|') for line in raw.splitlines())}
    assert set(counts) == set(names), {'expected': names, 'actual': counts}
    assert all(count == 1 for count in counts.values()), counts
    result = {'db_start_utc': start, 'test_path': '/umami-verb-test', 'test': 1,
              'http_status': statuses, 'db_events_with_test_marker': counts,
              'db_access': 'SELECT only; no deletions', 'user_agent': UA}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(f'{len(names)} new event names verified in DB with test:1; nothing deleted.')


if __name__ == '__main__':
    main()
