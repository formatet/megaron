// Guard: pages load fonts from our own server, never a third party, so a
// player's IP does not reach Google (Cinzel self-hosted 2026-11-22).
import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const webRoot = new URL('../../../../', import.meta.url).pathname; // web/
const fontsDir = join(webRoot, 'static/fonts');

function walk(dir) {
  return readdirSync(dir).flatMap(name => {
    const path = join(dir, name);
    if (name === 'node_modules') return [];
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
}

test('no page or stylesheet loads Google Fonts', () => {
  const hits = walk(webRoot)
    .filter(p => /\.(html|css|js|mjs)$/.test(p) && !p.endsWith('.test.mjs'))
    .filter(p => /fonts\.(googleapis|gstatic)\.com/.test(readFileSync(p, 'utf8')))
    .map(p => relative(webRoot, p));
  assert.deepEqual(hits, []);
});

test('every font file cinzel.css names exists', () => {
  const css = readFileSync(join(fontsDir, 'cinzel.css'), 'utf8');
  const urls = [...css.matchAll(/url\('([^']+)'\)/g)].map(m => m[1]);
  assert.ok(urls.length > 0);
  for (const u of urls) assert.ok(existsSync(join(fontsDir, u)), u);
});
