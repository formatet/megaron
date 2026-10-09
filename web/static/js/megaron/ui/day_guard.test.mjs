import test from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
const guard = new URL('../../../../../tools/day_guard.py', import.meta.url);
for (const surface of ['web', 'codex']) {
  test(`Day guard ${surface}: player units and wall clock stay distinct`, () => {
    const result = spawnSync('python3', [guard.pathname, surface], {encoding:'utf8'});
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /PASS/);
  });
}
