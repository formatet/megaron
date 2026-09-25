import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import {
  renderMarkdown, extractLinks, articleForKind, articleForDrawer, backlinksTo, searchArticles,
} from './codex.js';

// The Codex's promises (megaron_plan_kodex.md acceptance 4) are FUNCTION, so
// they are tests, not a glance: every notification kind the client can show
// has an article, every [[link]] and `related` entry resolves, and the index
// and the article files match one-to-one. A slice that adds a notification
// kind without an article goes red here.

const here = dirname(fileURLToPath(import.meta.url));
const codexDir = join(here, '../../../codex');
const index = JSON.parse(readFileSync(join(codexDir, 'index.json'), 'utf8'));
const ids = index.articles.map(a => a.id);
const bodies = Object.fromEntries(ids.map(id => [id, readFileSync(join(codexDir, id + '.md'), 'utf8')]));

// Every kind format.js knows: the notifIcon keys and the notifText case arms.
function clientKinds() {
  const src = readFileSync(join(here, 'format.js'), 'utf8');
  const kinds = new Set();
  const iconBlock = src.slice(src.indexOf('export function notifIcon'), src.indexOf("return icons[kind]"));
  for (const m of iconBlock.matchAll(/^\s+([A-Z][A-Za-z]+):/gm)) kinds.add(m[1]);
  const textFn = src.slice(src.indexOf('export function notifText'));
  for (const m of textFn.matchAll(/case '([A-Z][A-Za-z]+)'/g)) kinds.add(m[1]);
  return [...kinds];
}

test('codex: index ids are unique and every category is declared', () => {
  assert.equal(new Set(ids).size, ids.length);
  for (const a of index.articles) assert.ok(index.categories.includes(a.category), `${a.id}: unknown category ${a.category}`);
  for (const c of index.categories) assert.ok(index.articles.some(a => a.category === c), `empty category ${c}`);
});

test('codex: every index entry has a file and every file has an index entry', () => {
  const files = readdirSync(codexDir).filter(f => f.endsWith('.md')).map(f => f.slice(0, -3));
  assert.deepEqual(files.sort(), [...ids].sort());
  for (const id of ids) assert.ok(bodies[id].trim().length > 0, `${id}.md is empty`);
});

test('codex: every [[link]] and related entry resolves to an article', () => {
  for (const a of index.articles) {
    for (const l of extractLinks(bodies[a.id])) assert.ok(ids.includes(l), `${a.id}.md links to missing [[${l}]]`);
    for (const r of a.related || []) assert.ok(ids.includes(r), `${a.id}: related ${r} missing`);
  }
});

test('codex: every notification kind the client shows maps to exactly one article', () => {
  const kinds = clientKinds();
  assert.ok(kinds.length > 30, `parsed only ${kinds.length} kinds from format.js — parser broken?`);
  for (const k of kinds) assert.ok(articleForKind(k, index), `notification kind ${k} has no Codex article`);
  const seen = new Map();
  for (const a of index.articles) for (const k of a.kinds || []) {
    assert.ok(!seen.has(k), `${k} mapped twice (${seen.get(k)}, ${a.id})`);
    seen.set(k, a.id);
  }
});

test('codex: every drawer with a ? button has an article', () => {
  const html = readFileSync(join(here, '../../../map.html'), 'utf8');
  const drawers = [...html.matchAll(/openCodexForDrawer\('([a-z]+)'\)/g)].map(m => m[1]);
  assert.ok(drawers.length >= 8);
  for (const d of drawers) assert.ok(articleForDrawer(d, index), `drawer ${d} has no article`);
});

test('codex: every article is reachable — linked from another article or listed as related', () => {
  const reached = new Set(['welcome']);
  for (const a of index.articles) {
    extractLinks(bodies[a.id]).forEach(l => reached.add(l));
    (a.related || []).forEach(r => reached.add(r));
  }
  for (const id of ids) assert.ok(reached.has(id), `${id} is an orphan`);
});

test('renderMarkdown: escapes HTML and renders links, known and missing', () => {
  const html = renderMarkdown('A <b> and [[food]] and [[nope|gone]].', { food: 'Food and hunger' });
  assert.match(html, /&lt;b&gt;/);
  assert.match(html, /<a class="cx-link" href="#" data-cx="food">Food and hunger<\/a>/);
  assert.match(html, /<span class="cx-link-missing"[^>]*>gone<\/span>/);
});

test('renderMarkdown: headings, lists, tables, code and comments', () => {
  const md = [
    '<!-- src: somewhere.go -->',
    '## Head',
    '- one **bold**',
    '- two `a*b*c`',
    '',
    '| A | B |',
    '|---|---|',
    '| 1 | 2 |',
  ].join('\n');
  const html = renderMarkdown(md);
  assert.doesNotMatch(html, /somewhere/);
  assert.match(html, /<h3 class="cx-h">Head<\/h3>/);
  assert.match(html, /<li>one <strong>bold<\/strong><\/li>/);
  assert.match(html, /<code>a\*b\*c<\/code>/);
  assert.match(html, /<th>A<\/th><th>B<\/th>/);
  assert.match(html, /<td>1<\/td><td>2<\/td>/);
});

test('backlinksTo and searchArticles', () => {
  const b = { a: 'see [[c]]', b: 'nothing', c: 'self [[c]]' };
  assert.deepEqual(backlinksTo('c', b), ['a']);
  const idx = { articles: [{ id: 'a', title: 'Grain' }, { id: 'b', title: 'Wine' }] };
  assert.deepEqual(searchArticles('grain', idx, { a: '', b: 'made from grain' }).map(x => x.id), ['a', 'b']);
  assert.deepEqual(searchArticles('  ', idx, {}), []);
});
