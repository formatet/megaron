#!/usr/bin/env python3
"""Corrupt each new terrain table, require its DB-backed assertion, restore bytes."""
from pathlib import Path
import subprocess
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/todo24'
for slug in ['limestone-mountains', 'red-mountains', 'maquis', 'semi-desert', 'coastal-sea', 'deep-sea', 'rivers', 'river-ford']:
    path = ROOT / 'web/static/codex' / (slug + '.md')
    original = path.read_bytes()
    try:
        text = original.decode()
        start = text.index('<!-- terrain-production -->')
        marker = text.index(' |', text.index('| [[', start))
        text = text[:marker] + text[marker:].replace(' |', ' WRONG |', 1)
        path.write_text(text)
        result = subprocess.run(['tools/gotest.sh', './internal/economy', '-run', 'TestCodexTerrainProduction/' + slug + '$'], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (OUT / ('terrain-mutant-' + slug + '.log')).write_text(result.stdout)
        assert result.returncode != 0 and slug + ' production table differs from city rules' in result.stdout, result.stdout
        print(slug + ': killed', flush=True)
    finally:
        path.write_bytes(original)
    assert path.read_bytes() == original
