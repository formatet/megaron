import {execFileSync} from 'node:child_process';
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import {
  renderMarkdown, extractLinks, articleForKind, articleForDrawer, articleForEntity, backlinksTo, searchArticles,
  resolveCodexId, codexTitles, splitSections, articleContents, unlinkedMentions, linkFirstMentions,
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

// Every id a link may name: articles and their sections.
const addressable = Object.keys(codexTitles(index));
// text returns an article's body, or just its section for a section id.
function text(id) {
  const r = resolveCodexId(id, index);
  if (!r) return '';
  if (!r.section) return bodies[id];
  return splitSections(bodies[r.article.id]).filter(p => p.id === id).map(p => p.text).join('');
}
function topicBody(id) { const a=index.articles.find(a=>a.id===id);return [id,...(a?.related||[])].map(id=>bodies[id]||'').join('\n'); }

// Every kind format.js knows: the notifIcon keys and the notifText case arms.
function clientKinds() {
  const src = readFileSync(join(here, 'format.js'), 'utf8');
  const kinds = new Set();
  const iconBlock = src.slice(src.indexOf('export function notifIcon'), src.indexOf("return icons[kind]"));
  for (const m of iconBlock.matchAll(/^\s+([A-Za-z][A-Za-z0-9_]*):/gm)) kinds.add(m[1]);
  const textFn = src.slice(src.indexOf('export function notifText'), src.indexOf('// Mirrors keryx'));
  for (const m of textFn.matchAll(/^    case '([A-Za-z][A-Za-z0-9_]*)'/gm)) kinds.add(m[1]);
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
    for (const l of extractLinks(bodies[a.id])) assert.ok(addressable.includes(l), `${a.id}.md links to missing [[${l}]]`);
    for (const r of a.related || []) assert.ok(addressable.includes(r), `${a.id}: related ${r} missing`);
  }
});

test('codex: every notification kind the client shows maps to exactly one article', () => {
  const kinds = clientKinds();
  assert.ok(kinds.length > 30, `parsed only ${kinds.length} kinds from format.js — parser broken?`);
  for (const k of kinds) assert.ok(articleForKind(k, index), `notification kind ${k} has no Codex article`);
  const seen = new Map();
  for (const a of index.articles) for (const owner of [a, ...(a.sections || [])]) for (const k of owner.kinds || []) {
    assert.ok(!seen.has(k), `${k} mapped twice (${seen.get(k)}, ${owner.id})`);
    seen.set(k, owner.id);
  }
});

test('codex: every drawer with a ? button has an article', () => {
  const html = readFileSync(join(here, '../../../map.html'), 'utf8');
  const drawers = [...html.matchAll(/openCodexForDrawer\('([a-z]+)'\)/g)].map(m => m[1]);
  assert.ok(drawers.length >= 7); // Gossip's drawer was merged into Notifications
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
  assert.match(html, /<h3 class="cx-h" id="cx-s-h-head">Head<\/h3>/);
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

test('codex: lowercase agora_ready is mapped and community rules match alpha policy', () => {
  assert.ok(clientKinds().includes('agora_ready'));
  assert.equal(articleForKind('agora_ready', index), 'community-chat');
  const alpha = readFileSync(join(here, '../../../../templates/alfatestinfo.html'), 'utf8');
  const community = topicBody('community-chat');
  for (const text of ['https://discord.gg/NxtftQUMYW', 'Be decent.', 'Keep personal information private.', 'Contact a moderator']) {
    assert.ok(alpha.includes(text), text);
    assert.ok(community.includes(text), text);
  }
  for (const stale of ['Matrix', 'agora.formatet.se', 'Registration is closed']) {
    assert.ok(!alpha.includes(stale), stale);
    assert.ok(!community.includes(stale), stale);
  }

});

test('codex search finds the specific colony abandonment and Runner return articles', () => {
  assert.ok(searchArticles('abandon', index, bodies).some(a => a.id === 'colonies-abandoning-a-colony'), 'abandon must lead to the colony instructions');
  assert.ok(searchArticles('call-back', index, bodies).some(a => a.id === 'runner-return'), 'CLI call-back must lead to the passage instructions');
});

test('codex: game days name the world unit and clock/date names wall time', () => {
  assert.match(codexTitles(index)['time-game-days-and-the-clock'],/Game days and real time/);
  assert.match(topicBody('time'), /one \*\*game day\*\* at a time/);
  assert.doesNotMatch(bodies.time + bodies.welcome + bodies['coming-back'], /game[ -]game\b|\bticks?\b|nine hours/i);
  assert.match(bodies['coming-back'], /While you were away, the world kept moving/);
});


test('historical food-reserve notifications open the current food article', () => {
  // Historical only: these frozen kinds still exist in notification archives.
  for (const kind of ['SitosGranaryRelease', 'SitosIntervention', 'SitosFundLow']) {
    assert.equal(articleForKind(kind, index), 'food');
  }
});

test('temple demolition notice opens the loss and lowered-ceiling rules', () => {
  assert.equal(articleForKind('TempleDemolitionWrath', index), 'temple-demolition');
  const plain = text('temple-demolition').replaceAll('**', '');
  for (const pattern of [/10 kharis per temple level/, /floor of 1/, /lower ceiling takes no additional kharis/, /plundering/, /no demolition debt/]) assert.match(plain, pattern);
});

test('Codex search ranks titles before aliases before content and normalises separators',()=>{
 const idx={articles:[{id:'sea',title:'Sea',aliases:['ship'],search:'Galley carrier'},{id:'galley',title:'Galley',aliases:['ship'],search:'Timber'},{id:'timber',title:'Timber',search:'A galley needs wood'},{id:'kharis',title:'Kharis',aliases:['mana']}]};
 assert.deepEqual(searchArticles('galley',idx,{}).map(a=>a.id),['galley','sea','timber']);
 assert.deepEqual(searchArticles('SHIP',idx,{}).map(a=>a.id),['galley','sea']);
 assert.deepEqual(searchArticles('mana',idx,{}).map(a=>a.id),['kharis']);
 assert.deepEqual(searchArticles('bronze_guard',{articles:[{id:'elite-infantry',title:'Bronze Guard'}]},{}).map(a=>a.id),['elite-infantry']);
});


test('current catalogue objects resolve to their individual Codex article',()=>{
 assert.equal(articleForEntity('unit','merchantman',index),'emporos');
 assert.equal(articleForEntity('building','silo',index),'granary');
 assert.equal(articleForEntity('terrain','hills',index),'hills');
 assert.equal(articleForEntity('unit','trireme',index),null);
 assert.equal(articleForEntity('kingdom','oikos',index),null);
});
test('article bundle matches every Markdown source exactly',()=>{
 const bundle=JSON.parse(readFileSync(join(codexDir,'articles.json'),'utf8'));
 assert.deepEqual(Object.keys(bundle).sort(),[...ids].sort());
 for(const id of ids)assert.equal(bundle[id],bodies[id],id+' has a stale bundled article; run python3 tools/codex_facts.py');
});

test('generated Codex facts are current with their reference catalogue',()=>{
 execFileSync('python3',[join(here,'../../../../../tools/codex_facts.py'),'--check'],{stdio:'pipe'});
});


test('Codex tables keep labelled wiki links in one cell',()=>{
 const html=renderMarkdown('| Unit | Role |\n|---|---|\n| [[elite-infantry|Bronze Guard]] | Infantry |',{'elite-infantry':'Bronze Guard'});
 assert.equal((html.match(/<td>/g)||[]).length,2);
 assert.match(html,/<td><a[^>]*data-cx="elite-infantry">Bronze Guard<\/a><\/td>/);
});


test('Codex offer lifetime follows its server owner and distinguishes existing deadlines',()=>{
 const owner=readFileSync(join(here,'../../../../../server/internal/messenger/handler.go'),'utf8');
 const ticks=Number(/OfferExpiryTicks\s*=\s*(\d+)/.exec(owner)[1]);
 assert.ok(text('trade-offer-expiry').includes('**'+ticks+' game days after arrival**'));
 assert.match(text('trade-offer-expiry'),/already sent keep their recorded deadline/);
});

test('codex: every section heading anchor matches its index entry, and ids are unique across articles and sections', () => {
  const seen = new Set();
  for (const a of index.articles) {
    for (const id of [a.id, ...(a.sections || []).map(s => s.id)]) {
      assert.ok(!seen.has(id), `${id} is used twice`);
      seen.add(id);
    }
    const anchors = splitSections(bodies[a.id]).slice(1).map(p => p.id);
    for (const s of a.sections || []) assert.ok(anchors.includes(s.id) || /\{#[a-z0-9-]+\}/.test(bodies[a.id]) && bodies[a.id].includes('{#' + s.id + '}'), `${a.id}: section ${s.id} has no heading {#${s.id}}`);
    for (const m of bodies[a.id].matchAll(/\{#([a-z0-9-]+)\}/g)) assert.ok((a.sections || []).some(s => s.id === m[1]), `${a.id}: heading {#${m[1]}} is not listed under sections`);
  }
});

test('codex: an old fragment id opens its article at the section, and links to it render as links', () => {
  const r = resolveCodexId('colonies-abandoning-a-colony', index);
  assert.equal(r.article.id, 'colonies');
  assert.equal(r.section.id, 'colonies-abandoning-a-colony');
  const html = renderMarkdown('See [[colonies-abandoning-a-colony]].', codexTitles(index));
  assert.match(html, /data-cx="colonies-abandoning-a-colony">Abandoning a colony</);
  assert.match(renderMarkdown('## Abandoning a colony {#colonies-abandoning-a-colony}'), /<h3 class="cx-h" id="cx-s-colonies-abandoning-a-colony">Abandoning a colony<\/h3>/);
  assert.ok(backlinksTo('colonies', { colonies: '', other: 'see [[colonies-abandoning-a-colony]]' }, index).includes('other'));
});

test('codex: a long article opens with its contents, a short one does not', () => {
  assert.match(articleContents(bodies.runners), /data-cx-section="runner-return"/);
  assert.equal(articleContents('intro\n\n## One {#a}\n\nx'), '');
});

test('codex search finds a rule inside a long article as its section', () => {
  const hits = searchArticles('sailing time', index, bodies);
  assert.equal(hits[0].id, 'sailing-time');
  assert.match(hits[0].title, /›/);
});

test('codex: every article links the first mention of each thing that has its own page', () => {
  for (const a of index.articles) {
    const missing = unlinkedMentions(bodies[a.id], a.id, index);
    assert.deepEqual(missing.map(m => m.term + ' → ' + m.id), [], `${a.id} mentions without a link (run node tools/codex_links.mjs)`);
  }
});

test('linkFirstMentions links the first unlinked mention only, and leaves links, code, headings and generated tables alone', () => {
  const idx = { articles: [{ id: 'war-galley', title: 'War Galley', terms: ['War Galley'] }, { id: 'galley', title: 'Galley', terms: ['Galley'] }, { id: 'grain', title: 'Grain', terms: ['grain'] }] };
  const md = '## Galley\n\nA War Galley and a Galley. Another Galley. `grain`\n<!-- facts: unit x -->\n| grain |\n<!-- /facts -->';
  assert.equal(linkFirstMentions(md, 'self', idx), '## Galley\n\nA [[war-galley]] and a [[galley]]. Another Galley. `grain`\n<!-- facts: unit x -->\n| grain |\n<!-- /facts -->');
  assert.deepEqual(unlinkedMentions('See [[galley|the ship]]. A Galley.', 'self', idx), []);
  assert.deepEqual(unlinkedMentions('A Galley.', 'galley', idx), [], 'an article does not link to itself');
});

test('codex: every member page names a hub that exists and lists it', () => {
  for (const a of index.articles.filter(a => a.parent)) {
    const hub = resolveCodexId(a.parent, index);
    assert.ok(hub && !hub.section, `${a.id}: parent ${a.parent} is not an article`);
    assert.ok(extractLinks(bodies[a.parent]).includes(a.id), `${a.parent} does not list ${a.id}`);
  }
  assert.equal(index.articles.find(a => a.id === 'war-galley').parent, 'ship-types');
  assert.match(bodies['ship-types'], /\[\[galley\]\][\s\S]*\[\[war-galley\]\][\s\S]*\[\[emporos\]\]/);
});

test('founding article says keryx founding status shows the same forecast', () => {
  assert.match(bodies['founding'], /keryx founding status/);
});
