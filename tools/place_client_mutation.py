#!/usr/bin/env python3
"""Named red → restored green client guards for geographic naming."""
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
out = root / 'docs/reviews/flod-doparratt'
out.mkdir(parents=True, exist_ok=True)

def run(label, args):
    result = subprocess.run(args, cwd=root, capture_output=True, text=True)
    (out / (label + '.log')).write_text(result.stdout + result.stderr)
    return result

cases = [
    ('form-right', 'web/static/js/megaron/ui/placename.js', '!tile.can_name || ', '',
     ['node', '--test', 'web/static/js/megaron/ui/placename.test.mjs'],
     'place naming form is rights-gated'),
    ('notification-detail', 'server/cmd/keryx/cmd_notifications.go',
     'n.Kind == "PlaceFirstReached"', 'n.Kind == "XPlaceFirstReached"',
     ['go','-C','server','test','./cmd/keryx','-run','TestEveryNotificationKindHasReadableDetail/PlaceFirstReached','-count=1'],
     'notification kind PlaceFirstReached has no readable detail'),
]
for label, file, before, after, args, message in cases:
    path = root / file
    saved = path.read_bytes()
    try:
        text = saved.decode()
        if text.count(before) != 1:
            raise RuntimeError(f'{label}: mutation target must occur once')
        path.write_text(text.replace(before, after))
        result = run('mutation-client-' + label, args)
        if result.returncode == 0 or message not in result.stdout + result.stderr:
            raise RuntimeError(f'{label}: mutation failed to produce named red')
        print(label + ': named red')
    finally:
        path.write_bytes(saved)
    result = run('restored-client-' + label, args)
    if result.returncode:
        raise RuntimeError(f'{label}: restored run is not green')
    print(label + ': restored green')
