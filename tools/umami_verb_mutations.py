#!/usr/bin/env python3
"""Physically remove a catalog row, the success track, or API wiring; restore each."""
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/umami-verb'
CATALOG = ROOT / 'web/static/js/megaron/verb_telemetry.js'
API = ROOT / 'web/static/js/megaron/api.js'
TEST = ['node', '--test', 'web/static/js/megaron/verb_telemetry.test.mjs']


def run():
    original = CATALOG.read_bytes()
    api_original = API.read_bytes()
    row = next(line for line in original.decode().splitlines(True) if '"verb": "build"' in line)
    cases = [
        ('catalog-row', CATALOG, row, '', 'Umami parity: every mutating vault verb'),
        ('success-track', CATALOG, '      track(entry.event, props);', '', 'Umami success: build'),
        ('api-wiring', API, '      trackVerbResponse(url, opts, res);', '', 'Umami success: build'),
    ]
    try:
        for name, path, before, after, expected in cases:
            source = path.read_text()
            assert source.count(before) == 1, name
            path.write_text(source.replace(before, after))
            result = subprocess.run(TEST, cwd=ROOT, text=True, capture_output=True, timeout=30)
            (OUT / f'mutation-{name}.log').write_text(result.stdout + result.stderr)
            assert result.returncode != 0 and any(
                line.startswith('not ok') and expected in line for line in result.stdout.splitlines()
            ), (name, result.stdout)
            print(name + ': named test RED')
            CATALOG.write_bytes(original)
            API.write_bytes(api_original)
    finally:
        CATALOG.write_bytes(original)
        API.write_bytes(api_original)
    result = subprocess.run(TEST, cwd=ROOT, text=True, capture_output=True, timeout=30)
    (OUT / 'restored-js.log').write_text(result.stdout + result.stderr)
    assert result.returncode == 0
    print('restored: GREEN (byte-identical source)')


if __name__ == '__main__':
    run()
