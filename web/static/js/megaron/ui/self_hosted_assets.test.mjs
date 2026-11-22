// Guard: pages load fonts, scripts and stylesheets from our own server, never a
// third party, so a player's IP does not reach Google or a CDN (Cinzel and htmx
// self-hosted 2026-11-22). Timothy's own hosts (formatet.se) are allowed.
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

// Hosts we run ourselves; anything else in a <script src> or stylesheet <link> fails.
const OWN_HOSTS = /^https?:\/\/([a-z0-9-]+\.)*formatet\.se\//;

test('no page loads a script or stylesheet from a third-party host', () => {
  const hits = [];
  for (const p of walk(webRoot).filter(p => p.endsWith('.html'))) {
    const html = readFileSync(p, 'utf8');
    const tags = [...html.matchAll(/<script\b[^>]*\bsrc=["']([^"']+)["']/g),
                  ...html.matchAll(/<link\b[^>]*\bhref=["']([^"']+)["'][^>]*>/g)]
      .filter(m => !m[0].startsWith('<link') || /rel=["'](stylesheet|preload|preconnect|modulepreload)["']/.test(m[0]));
    for (const m of tags) {
      if (/^https?:\/\//.test(m[1]) && !OWN_HOSTS.test(m[1])) hits.push(`${relative(webRoot, p)}: ${m[1]}`);
    }
  }
  assert.deepEqual(hits, []);
});
