// Guard: player-visible numbers carry at most one decimal (Timothy 2026-11-18).
// Scans web JS for toFixed(2..9) / maximumFractionDigits 2..9 and the Codex for
// numbers with two or more decimals. Use fmtNum / fmtTenths (fmt_num.js) instead.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const jsRoot = new URL('../../', import.meta.url).pathname;
const codexRoot = new URL('../../../codex/', import.meta.url).pathname;

// file (relative to web/static/js) -> reason it may keep a long decimal. Empty: no exceptions.
const JS_EXCEPTIONS = {};
// Codex files exempt from the two-decimal scan, with reason. Empty: no exceptions.
const CODEX_EXCEPTIONS = {};

function walk(dir) {
  return readdirSync(dir).flatMap(name => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
}

test('web JS never prints more than one decimal', () => {
  const hits = [];
  for (const file of walk(jsRoot).filter(f => f.endsWith('.js'))) {
    const rel = relative(jsRoot, file);
    if (rel in JS_EXCEPTIONS) continue;
    readFileSync(file, 'utf8').split('\n').forEach((line, i) => {
      if (/toFixed\([2-9]\)|maximumFractionDigits: ?[2-9]/.test(line)) hits.push(`${rel}:${i + 1}: ${line.trim().slice(0, 100)}`);
    });
  }
  assert.deepEqual(hits, []);
});

test('Codex articles never print a number with two decimals', () => {
  const hits = [];
  for (const name of readdirSync(codexRoot).filter(f => f.endsWith('.md'))) {
    if (name in CODEX_EXCEPTIONS) continue;
    readFileSync(join(codexRoot, name), 'utf8').split('\n').forEach((line, i) => {
      if (/\d\.\d{2,}/.test(line)) hits.push(`${name}:${i + 1}: ${line.trim().slice(0, 100)}`);
    });
  }
  assert.deepEqual(hits, []);
});

import { fmtNum, fmtTenths, fmtDuration } from './fmt_num.js';
import { kx } from './drawers/kult_kharis.js';

test('one-decimal formatters: -0.33 reads -0.3, 0.04 reads <0.1', () => {
  assert.equal(kx(-0.33), '-0.3');
  assert.equal(kx(0.04), '<0.1');
  assert.equal(kx(2.34), '2.3');
  assert.equal(fmtTenths(-0.04), '−<0.1');
  assert.equal(fmtTenths(0), '0');
  assert.equal(fmtNum(-0.33), '-0.3');
  assert.match(fmtDuration(0.01, 1800), /about 0\.3 minutes/);
  assert.match(fmtDuration(0.0001, 1800), /less than 0\.1 minute/);
});
