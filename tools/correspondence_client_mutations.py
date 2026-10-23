#!/usr/bin/env python3
"""Physical K1 client source mutants; require named red and exact restore green.
Run only in owned worktree without concurrent browser/JS consumer tests.
"""
from pathlib import Path
import hashlib
import json
import subprocess

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/correspondence-k1'
OUT.mkdir(parents=True, exist_ok=True)
results = []
for name, path, old, new, command, cwd in [
    ('cli-wrong-queue-path', 'server/cmd/keryx/cmd_correspondence_queue.go', '/messengers/queue?%s', '/messengers/inbox?%s',
     ['go', 'test', './cmd/keryx', '-run', 'TestInboxQueueUsesServerActions', '-count=1'], ROOT / 'server'),
    ('web-eager-hidden-history', 'web/static/js/megaron/ui/drawers/diplomacy.js', '  await loadDipQueue();', '  await Promise.all([loadDipQueue(), loadDipThreads()]);',
     ['node', '--experimental-default-type=module', '--test', '--test-name-pattern', 'Queue mount does not fetch legacy history', 'web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'], ROOT),
    ('web-invents-accept', 'web/static/js/megaron/ui/drawers/diplomacy_queue.js', 'const supplied = Array.isArray(item.actions) ? item.actions : [];',
     "const supplied = ['read', 'reply', 'trade_accept', 'trade_decline'];",
     ['node', '--experimental-default-type=module', '--test', 'web/static/js/megaron/ui/drawers/diplomacy_queue.test.mjs'], ROOT),
]:
    source = ROOT / path
    original = source.read_bytes()
    assert old in original.decode()
    sha = hashlib.sha256(original).hexdigest()
    try:
        source.write_text(original.decode().replace(old, new, 1))
        result = subprocess.run(command, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (OUT / (name + '-red.log')).write_text(result.stdout)
        assert result.returncode != 0, name + ' survived'
    finally:
        source.write_bytes(original)
    assert hashlib.sha256(source.read_bytes()).hexdigest() == sha
    result = subprocess.run(command, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (OUT / (name + '-restored.log')).write_text(result.stdout)
    assert result.returncode == 0, result.stdout
    results.append(dict(mutation=name, source=path, restore_sha256=sha, mutant='RED', restored='GREEN'))
(OUT / 'client-mutations.json').write_text(json.dumps(results, indent=2))
print('All three client mutations RED and SHA-exact restoration GREEN')
