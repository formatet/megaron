#!/usr/bin/env node
// Link the first mention of every Codex article an article names but never
// links (rules: unlinkedMentions in web/static/js/megaron/ui/codex.js).
// Then run `python3 tools/codex_facts.py` to rebuild the article bundle.
// --check lists what would change and exits 1, writing nothing.
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { unlinkedMentions, linkFirstMentions } from '../web/static/js/megaron/ui/codex.js';

const dir = join(dirname(fileURLToPath(import.meta.url)), '../web/static/codex');
const idx = JSON.parse(readFileSync(join(dir, 'index.json'), 'utf8'));
const check = process.argv.includes('--check');
let n = 0;
for (const a of idx.articles) {
  const path = join(dir, a.id + '.md');
  const md = readFileSync(path, 'utf8');
  const missing = unlinkedMentions(md, a.id, idx);
  if (!missing.length) continue;
  n += missing.length;
  if (check) console.log(a.id + ': ' + missing.map(m => m.term + ' → ' + m.id).join(', '));
  else writeFileSync(path, linkFirstMentions(md, a.id, idx));
}
console.log((check ? 'Unlinked mentions: ' : 'Linked: ') + n);
if (check && n) process.exit(1);
