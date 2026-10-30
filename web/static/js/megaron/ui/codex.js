import { openMobilePanel, closeMobilePanel } from './mobile_panels.js';
// ── Codex — the player's wiki inside the game (megaron_plan_kodex.md) ──────
// Timothy 2026-09-24: "bygg in manualen i spelet — uppe till höger som en
// ikon, organiserad på wiki-sätt snarare än som en manual, och länkad från
// notiser osv."
//
// Content lives next to the code that shows it: web/static/codex/index.json
// (metadata: id, title, category, which notification kinds and which drawer
// point at the article) + one <id>.md per article (body only). A slice that
// changes a verb changes its article in the same commit. codex.test.mjs
// proves every notification kind is mapped and every [[link]] resolves.
//
// Three doors, one panel: the ? in the top bar (index + search), "Read about
// this" in the dispatch window (ui/dispatch_window.js), and the ? in every
// drawer header. The panel is anchored LEFT and sits outside the drawer
// system, so the City drawer can stay open while you read about placement.

// The name on the surface is terminology, and terminology is Timothy's call
// (megaron_beslut_infor_playtest.md P6) — one constant so it changes in one place.
const CODEX_NAME = 'Codex';

const BASE = '/static/codex/';

let index = null;        // { categories: [...], articles: [...] } once loaded
let bodies = null;       // id → markdown, once every article is fetched
let trail = [];        // article ids visited, for Back
let loading = null;

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ── Pure helpers (exported for codex.test.mjs) ───────────────────────────

// extractLinks returns the article ids a body links to, in order, deduped.
export function extractLinks(md) {
  const out = [];
  for (const m of String(md).matchAll(/\[\[([a-z0-9-]+)(?:\|[^\]]*)?\]\]/g)) {
    if (!out.includes(m[1])) out.push(m[1]);
  }
  return out;
}

// articleForKind maps a notification kind to the article that explains it,
// or null. Reads the index's `kinds` lists — the only place the mapping lives.
export function articleForKind(kind, idx = index) {
  if (!idx || !kind) return null;
  for (const a of idx.articles) {
    if ((a.kinds || []).includes(kind)) return a.id;
    const s = (a.sections || []).find(s => (s.kinds || []).includes(kind));
    if (s) return s.id;
  }
  return null;
}

// resolveCodexId maps an id to the article that holds it: an article's own id,
// or a section id (a heading `## Title {#id}` listed under the article's
// `sections`). Old ids from the 2026-10-28 split live on as section ids, so
// every link, notification and keryx command that named one still lands.
export function resolveCodexId(id, idx = index) {
  if (!idx || !id) return null;
  for (const a of idx.articles) {
    if (a.id === id) return { article: a, section: null };
    const s = (a.sections || []).find(s => s.id === id);
    if (s) return { article: a, section: s };
  }
  return null;
}

// codexTitles maps every addressable id to the name a link shows: the
// article's title, or the section's own heading.
export function codexTitles(idx = index) {
  const out = {};
  for (const a of (idx ? idx.articles : [])) {
    out[a.id] = a.title;
    for (const s of a.sections || []) out[s.id] = s.title;
  }
  return out;
}

// splitSections cuts an article body at its ## headings: the intro first
// (id null), then one part per heading, ### subsections included.
export function splitSections(md) {
  const parts = [{ id: null, title: '', text: '' }];
  for (const line of String(md).split('\n')) {
    const h = line.match(/^##\s+(.*?)(?:\s+\{#([a-z0-9-]+)\})?\s*$/);
    if (h) { parts.push({ id: h[2] || slug(h[1]), title: h[1], text: '' }); continue; }
    parts[parts.length - 1].text += line + '\n';
  }
  return parts;
}

function slug(text) {
  return 'h-' + String(text).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

export function articleForDrawer(drawer, idx = index) {
  if (!idx || !drawer) return null;
  const a = idx.articles.find(a => a.drawer === drawer);
  return a ? a.id : null;
}

export function articleForEntity(kind,key,idx=index) {
 return idx?.articles.find(a=>!a.future&&(a.entities||[]).some(e=>e.kind===kind&&e.key===key))?.id||null;
}
async function openCodexForEntity(kind,key) {
 await ensureLoaded();const id=articleForEntity(kind,key);if(id)await openCodex(id);
}

// inline renders the inline subset: `code`, **bold**, *italic*, [[id]] and
// [[id|text]]. Input is escaped first; code spans are protected from the
// other rules so `a*b` inside backticks stays literal.
function inline(text, titles) {
  const codes = [];
  let s = esc(text).replace(/`([^`]+)`/g, (_, c) => {
    codes.push(c);
    return '\u0000' + (codes.length - 1) + '\u0000';
  });
  s = s.replace(/\[\[([a-z0-9-]+)(?:\|([^\]]*))?\]\]/g, (_, id, label) => {
    const known = titles && titles[id];
    const text = label || known || id;
    return known
      ? `<a class="cx-link" href="#" data-cx="${id}">${text}</a>`
      : `<span class="cx-link-missing" title="No such article">${text}</span>`;
  });
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/(^|[^*])\*([^*\s][^*]*)\*/g, '$1<em>$2</em>');
  return s.replace(/\u0000(\d+)\u0000/g, (_, i) => `<code>${codes[Number(i)]}</code>`);
}

// renderMarkdown renders the small markdown subset the articles use:
// ## / ### headings, paragraphs, - and 1. lists, > quotes, ``` blocks,
// | tables |, and the inline rules above. HTML comments (used to cite the
// code a number was read from) are dropped. `titles` maps id → title so
// links can show the article's name and unknown ids render as missing.
export function renderMarkdown(md, titles = {}) {
  const lines = String(md).replace(/<!--[\s\S]*?-->/g, '').split('\n');
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }
    if (line.startsWith('```')) {
      const buf = [];
      i++;
      while (i < lines.length && !lines[i].startsWith('```')) buf.push(lines[i++]);
      i++;
      out.push(`<pre class="cx-pre">${esc(buf.join('\n'))}</pre>`);
      continue;
    }
    const h = line.match(/^(#{2,3})\s+(.*?)(?:\s+\{#([a-z0-9-]+)\})?\s*$/);
    if (h) {
      const tag = h[1].length === 2 ? 'h3' : 'h4';
      const anchor = h[3] || slug(h[2]);
      out.push(`<${tag} class="cx-h" id="cx-s-${anchor}">${inline(h[2], titles)}</${tag}>`);
      i++;
      continue;
    }
    if (line.startsWith('|')) {
      const rows = [];
      while (i < lines.length && lines[i].startsWith('|')) rows.push(lines[i++]);
      const cells = r => r.replace(/^\||\|$/g, '').replace(/\[\[([^\]]+)\]\]/g, link => link.replaceAll('|', '\u0001')).split('|').map(c => c.trim().replaceAll('\u0001', '|'));
      const body = rows.filter(r => !/^\|[\s:|-]+\|?$/.test(r));
      const [head, ...rest] = body;
      out.push('<table class="cx-table"><tr>' +
        cells(head).map(c => `<th>${inline(c, titles)}</th>`).join('') + '</tr>' +
        rest.map(r => '<tr>' + cells(r).map(c => `<td>${inline(c, titles)}</td>`).join('') + '</tr>').join('') +
        '</table>');
      continue;
    }
    const list = line.match(/^(\s*)(-|\d+\.)\s+/);
    if (list) {
      const ordered = list[2] !== '-';
      const items = [];
      while (i < lines.length) {
        const m = lines[i].match(/^\s*(-|\d+\.)\s+(.*)$/);
        if (m) { items.push(m[2]); i++; continue; }
        // A continuation line (indented, non-empty) belongs to the last item.
        if (items.length && /^\s+\S/.test(lines[i])) { items[items.length - 1] += ' ' + lines[i].trim(); i++; continue; }
        break;
      }
      const tag = ordered ? 'ol' : 'ul';
      out.push(`<${tag} class="cx-list">` + items.map(t => `<li>${inline(t, titles)}</li>`).join('') + `</${tag}>`);
      continue;
    }
    if (line.startsWith('>')) {
      const buf = [];
      while (i < lines.length && lines[i].startsWith('>')) buf.push(lines[i++].replace(/^>\s?/, ''));
      out.push(`<blockquote class="cx-quote">${inline(buf.join(' '), titles)}</blockquote>`);
      continue;
    }
    const buf = [];
    while (i < lines.length && lines[i].trim() && !/^(#{2,3}\s|```|\||>|\s*(-|\d+\.)\s)/.test(lines[i])) {
      buf.push(lines[i++].trim());
    }
    out.push(`<p>${inline(buf.join(' '), titles)}</p>`);
  }
  return out.join('\n');
}

// ── Link discipline ─────────────────────────────────────────────────────
// A wiki links the first mention of every thing that has its own page. Each
// article lists the words that mean it in prose (`terms` in index.json: "War
// Galley", "grain", "Runner"). unlinkedMentions finds the things an article
// mentions but never links to; codex.test.mjs keeps that list empty, and
// tools/codex_links.mjs links the first mention when it is not.

// prose blanks out what is not running text — links, code, comments,
// headings — keeping offsets, so a match index points into the original.
function prose(md) {
  const blank = m => m.replace(/[^\n]/g, ' ');
  return String(md)
    // Generated by tools/codex_facts.py; a link written there would be overwritten.
    .replace(/<!-- facts:[\s\S]*?<!-- \/facts -->/g, blank)
    .replace(/<!-- list:[\s\S]*?<!-- \/list -->/g, blank)
    .replace(/^\| \*\*(?:Cost for one (?:cohort|ship)|Training|Build time)\*\* \|.*$/gm, blank)
    .replace(/<!--[\s\S]*?-->/g, blank)
    .replace(/\[\[[^\]]*\]\]/g, blank)
    .replace(/`[^`]*`/g, blank)
    .replace(/^#{1,6} .*$/gm, blank);
}

function termPattern(idx) {
  const terms = [];
  for (const a of idx.articles) for (const t of a.terms || []) terms.push([t, a.id]);
  terms.sort((x, y) => y[0].length - x[0].length);
  const owner = new Map(terms.map(([t, id]) => [t, id]));
  const re = new RegExp("(?<![A-Za-z'’-])(" + terms.map(([t]) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|') + ")(?![A-Za-z])", 'g');
  return { re, owner };
}

// unlinkedMentions returns [{id, term, at}] — the first mention of each other
// article this body names in prose but never links (to it or to a section).
export function unlinkedMentions(md, selfId, idx = index) {
  if (!idx) return [];
  const linked = new Set();
  for (const l of extractLinks(md)) { const r = resolveCodexId(l, idx); if (r) linked.add(r.article.id); }
  const { re, owner } = termPattern(idx);
  const out = new Map();
  for (const m of prose(md).matchAll(re)) {
    const id = owner.get(m[1]);
    if (id === selfId || linked.has(id) || out.has(id)) continue;
    out.set(id, { id, term: m[1], at: m.index });
  }
  return [...out.values()];
}

// linkFirstMentions links each unlinked mention found above, in place.
export function linkFirstMentions(md, selfId, idx = index) {
  const titles = codexTitles(idx);
  let out = String(md);
  for (const { id, term, at } of unlinkedMentions(md, selfId, idx).sort((x, y) => y.at - x.at)) {
    const link = titles[id] === term ? `[[${id}]]` : `[[${id}|${term}]]`;
    out = out.slice(0, at) + link + out.slice(at + term.length);
  }
  return out;
}

// backlinksTo lists the articles whose body links to id — the wiki's
// "Linked from", computed rather than maintained by hand.
// A link to one of the article's sections counts as a link to the article.
export function backlinksTo(id, allBodies, idx = index) {
  const a = idx && idx.articles.find(a => a.id === id);
  const own = new Set([id, ...((a && a.sections) || []).map(s => s.id)]);
  return Object.keys(allBodies).filter(other => other !== id && extractLinks(allBodies[other]).some(l => own.has(l)));
}

// searchArticles ranks articles and their sections for a query: title hits
// first, then aliases, then body text. An article's body here is its intro;
// each section is searched on its own, so a rule deep in a long article is
// found as that section ("Ships › Sailing time"), not as the whole article.
export function searchArticles(q, idx, allBodies) {
  const normalise = value => String(value || '').toLowerCase().replace(/[_-]+/g,' ').replace(/\s+/g,' ').trim();
  const needle=normalise(q);if(!needle||!idx)return [];
  const docs=[];
  for(const a of idx.articles){
    const parts=splitSections(allBodies?.[a.id]||a.search||'');
    docs.push({a,title:a.title,aliases:a.aliases,body:parts[0].text});
    for(const s of a.sections||[]){
      const part=parts.find(p=>p.id===s.id);
      docs.push({a:{...s,title:a.title+' › '+s.title,future:a.future},title:s.title,aliases:s.aliases,body:part?part.text:''});
    }
  }
  return docs.map(d=>{
    const title=normalise(d.title),aliases=(d.aliases||[]).map(normalise),body=normalise(d.body);
    const score=title===needle?0:title.includes(needle)?1:aliases.some(alias=>alias.includes(needle))?2:body.includes(needle)?3:4;
    return {a:d.a,score};
  }).filter(row=>row.score<4).sort((x,y)=>x.score-y.score||x.a.title.localeCompare(y.a.title)).map(row=>row.a);
}

// ── Loading ──────────────────────────────────────────────────────────────

// initCodex fetches only the index at startup, so the dispatch window and
// drawer ? buttons know synchronously which articles exist. Bodies are
// fetched on first open.
export function initCodex() {
  document.addEventListener('click', onPanelClick);
  fetch(BASE + 'index.json')
    .then(r => r.ok ? r.json() : null)
    .then(d => { if (d) index = d; })
    .catch(() => {});
}

export function codexArticleForKind(kind) { return articleForKind(kind); }

async function ensureLoaded() {
  if (index && bodies) return;
  if (!loading) {
    loading = (async () => {
      if (!index) {
        const r = await fetch(BASE + 'index.json');
        index = await r.json();
      }
      const response=await fetch(BASE+'articles.json');
      if(!response.ok)throw new Error('Could not load Codex articles');
      const bundle=await response.json();
      if(!index.articles.every(a=>typeof bundle[a.id]==='string'))throw new Error('Incomplete Codex articles');
      bodies=bundle;
    })();
  }
  try { await loading; } catch (_) { loading = null; throw _; }
}

function titles() {
  return codexTitles(index);
}

// ── Panel ────────────────────────────────────────────────────────────────

function panel() { return document.getElementById('codex-panel'); }

export async function openCodex(id) {
  const el = panel();
  if (!el) return;
  openMobilePanel(el, closeCodex);
  el.classList.add('open');
  const body = document.getElementById('codex-body');
  if (!bodies) body.innerHTML = '<div class="loading">Loading…</div>';
  try {
    await ensureLoaded();
  } catch (_) {
    body.innerHTML = `<p class="empty-state">The ${CODEX_NAME} could not be loaded.</p>`;
    return;
  }
  if (id && resolveCodexId(id)) {
    if (trail[trail.length - 1] !== id) trail.push(id);
    renderArticle(id);
  } else {
    trail = [];
    renderIndex();
  }
}

export function openCodexForDrawer(drawer) {
  openCodex(articleForDrawer(drawer));
}

export function closeCodex() {
  const el = panel();
  closeMobilePanel(el);
  if (el) el.classList.remove('open');
}

export function codexBack() {
  trail.pop();
  const prev = trail[trail.length - 1];
  if (prev) renderArticle(prev); else renderIndex();
}

function codexSearch(q) {
  const out = document.getElementById('codex-results');
  if (!out) return;
  if (!q.trim()) { out.innerHTML = ''; return; }
  const hits = searchArticles(q, index, bodies);
  out.innerHTML = hits.length
    ? hits.map(a => `<a class="cx-link cx-result" href="#" data-cx="${a.id}">${esc(a.title)}${a.future?' <small>(future)</small>':''}</a>`).join('')
    : '<p class="empty-state">Nothing matches.</p>';
}

function setHeader(title, canGoBack) {
  document.getElementById('codex-title').textContent = title;
  document.getElementById('codex-back').classList.toggle('hidden', !canGoBack);
}

function renderIndex() {
  setHeader(CODEX_NAME, false);
  const body = document.getElementById('codex-body');
  const cats = index.categories.map(cat => {
    const items = index.articles.filter(a => a.category === cat);
    return `<div class="cx-cat"><div class="cx-cat-title">${esc(cat)}</div>` +
      items.map(a => `<a class="cx-link cx-entry" href="#" data-cx="${a.id}">${esc(a.title)}${a.future?' <small>(future)</small>':''}</a>`).join('') +
      '</div>';
  }).join('');
  body.innerHTML =
    `<input class="cx-search" id="codex-search" type="search" placeholder="Search the ${CODEX_NAME}…" autocomplete="off">` +
    '<div id="codex-results" class="cx-results"></div>' +
    `<div class="cx-index">${cats}</div>`;
  const input = document.getElementById('codex-search');
  input.addEventListener('input', () => codexSearch(input.value));
  body.scrollTop = 0;
}

// articleContents lists an article's ## sections as links — the table of
// contents a long article opens with. Short articles get none.
export function articleContents(md, minSections = 3) {
  const parts = splitSections(md).slice(1);
  if (parts.length < minSections) return '';
  return '<nav class="cx-toc"><span class="cx-foot-label">Contents</span>' +
    parts.map(p => `<a class="cx-link" href="#" data-cx-section="${p.id}">${esc(p.title)}</a>`).join('') + '</nav>';
}

function renderArticle(target) {
  const { article: a, section } = resolveCodexId(target);
  const id = a.id;
  setHeader(a.title, true);
  const t = titles();
  const back = backlinksTo(id, bodies);
  const related = (a.related || []).filter(r => t[r]);
  const hub = a.parent && resolveCodexId(a.parent);
  const body = document.getElementById('codex-body');
  body.innerHTML =
    `<div class="cx-crumb">${esc(a.category)}` +
      (hub ? ` › <a class="cx-link" href="#" data-cx="${a.parent}">${esc(t[a.parent])}</a>` : '') + '</div>' +
    articleContents(bodies[id] || '') +
    `<article class="cx-article">${renderMarkdown(bodies[id] || '', t)}</article>` +
    (related.length ? '<div class="cx-foot"><span class="cx-foot-label">See also</span> ' +
      related.map(r => `<a class="cx-link" href="#" data-cx="${r}">${esc(t[r])}</a>`).join(' · ') + '</div>' : '') +
    (back.length ? '<div class="cx-foot"><span class="cx-foot-label">Linked from</span> ' +
      back.map(r => `<a class="cx-link" href="#" data-cx="${r}">${esc(t[r])}</a>`).join(' · ') + '</div>' : '');
  body.scrollTop = 0;
  if (section) scrollToSection(section.id);
}

function scrollToSection(sectionId) {
  const body = document.getElementById('codex-body');
  const h = document.getElementById('cx-s-' + sectionId);
  if (!body || !h) return;
  body.scrollTop = h.offsetTop - body.offsetTop;
  h.classList.add('cx-target');
  setTimeout(() => h.classList.remove('cx-target'), 1600);
}

// One delegated listener for every link the panel ever renders. Bound in
// initCodex, not at module top level: dispatch_window.js imports this module,
// and a top-level DOM touch would run in every module (and test) that
// imports the dispatch window — the fragility ws.js's header warns about.
function onPanelClick(e) {
  const entity=e.target.closest?.('[data-cx-kind][data-cx-key]');
  if(entity){e.preventDefault();openCodexForEntity(entity.dataset.cxKind,entity.dataset.cxKey);return;}
  const toc = e.target.closest && e.target.closest('#codex-panel [data-cx-section]');
  if (toc) { e.preventDefault(); scrollToSection(toc.dataset.cxSection); return; }
  const a = e.target.closest && e.target.closest('#codex-panel [data-cx]');
  if (!a) return;
  e.preventDefault();
  openCodex(a.dataset.cx);
}
