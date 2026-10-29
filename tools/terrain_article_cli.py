#!/usr/bin/env python3
"""Read new terrain articles through installed Keryx and isolated static HTTP."""
import functools
import http.server
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading

root = Path(__file__).resolve().parents[1]
class Quiet(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Quiet, directory=str(root / 'web')))
threading.Thread(target=server.serve_forever, daemon=True).start()
logs = []
try:
    with tempfile.TemporaryDirectory() as tmp:
        cfg = Path(tmp) / 'config.json'
        cfg.write_text(json.dumps({'server': f'http://127.0.0.1:{server.server_port}', 'world_id': '00000000-0000-0000-0000-000000000001'}))
        env = {'HOME': os.environ['HOME'], 'PATH': os.environ['PATH'], 'POLEIA_CONFIG': str(cfg)}
        for slug in ['limestone-mountains', 'red-mountains', 'maquis', 'semi-desert', 'coastal-sea', 'deep-sea', 'rivers', 'river-ford']:
            result = subprocess.run([str(Path.home() / 'go/bin/poleia'), 'codex', slug], env=env, text=True, capture_output=True)
            assert result.returncode == 0 and 'Base production' in result.stdout, (slug, result.stderr)
            logs.append(result.stdout)
finally:
    server.shutdown()
(root / 'docs/reviews/todo24/terrain-keryx.log').write_text('\n'.join(logs))
print('8 Keryx Codex reads passed against isolated static HTTP server')
