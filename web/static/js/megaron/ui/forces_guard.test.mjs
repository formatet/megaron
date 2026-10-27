import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

test('retired Forces labels cannot return to player literals or Codex titles',()=>{
 const guard=fileURLToPath(new URL('../../../../../tools/forces_guard.py',import.meta.url));
 const output=execFileSync('python3',[guard],{encoding:'utf8'});
 assert.match(output,/Forces player labels: PASS/);
});
