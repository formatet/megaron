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
export const CODEX_NAME = 'Codex';

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
  const a = idx.articles.find(a => (a.kinds || []).includes(kind));
  return a ? a.id : null;
}

export function articleForDrawer(drawer, idx = index) {
  if (!idx || !drawer) return null;
  const a = idx.articles.find(a => a.drawer === drawer);
  return a ? a.id : null;
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
    const h = line.match(/^(#{2,3})\s+(.*)$/);
    if (h) {
      const tag = h[1].length === 2 ? 'h3' : 'h4';
      out.push(`<${tag} class="cx-h">${inline(h[2], titles)}</${tag}>`);
      i++;
      continue;
    }
    if (line.startsWith('|')) {
      const rows = [];
      while (i < lines.length && lines[i].startsWith('|')) rows.push(lines[i++]);
      const cells = r => r.replace(/^\||\|$/g, '').split('|').map(c => c.trim());
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

// backlinksTo lists the articles whose body links to id — the wiki's
// "Linked from", computed rather than maintained by hand.
export function backlinksTo(id, allBodies) {
  return Object.keys(allBodies).filter(other => other !== id && extractLinks(allBodies[other]).includes(id));
}

// searchArticles ranks articles for a query: title hits first, then body hits.
export function searchArticles(q, idx, allBodies) {
  const needle = q.trim().toLowerCase();
  if (!needle || !idx) return [];
  const scored = [];
  for (const a of idx.articles) {
    const inTitle = a.title.toLowerCase().includes(needle);
    const inBody = (allBodies && allBodies[a.id] || '').toLowerCase().includes(needle);
    if (inTitle || inBody) scored.push({ a, score: inTitle ? 0 : 1 });
  }
  return scored.sort((x, y) => x.score - y.score || x.a.title.localeCompare(y.a.title)).map(s => s.a);
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
      const entries = await Promise.all(index.articles.map(async a => {
        try {
          const r = await fetch(BASE + a.id + '.md');
          return [a.id, r.ok ? await r.text() : ''];
        } catch (_) { return [a.id, '']; }
      }));
      bodies = Object.fromEntries(entries);
    })();
  }
  try { await loading; } catch (_) { loading = null; throw _; }
}

function titles() {
  return Object.fromEntries((index ? index.articles : []).map(a => [a.id, a.title]));
}

// ── Panel ────────────────────────────────────────────────────────────────

function panel() { return document.getElementById('codex-panel'); }

export async function openCodex(id) {
  const el = panel();
  if (!el) return;
  el.classList.add('open');
  const body = document.getElementById('codex-body');
  if (!bodies) body.innerHTML = '<div class="loading">Loading…</div>';
  try {
    await ensureLoaded();
  } catch (_) {
    body.innerHTML = `<p class="empty-state">The ${CODEX_NAME} could not be loaded.</p>`;
    return;
  }
  if (id && index.articles.some(a => a.id === id)) {
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
  if (el) el.classList.remove('open');
}

export function codexBack() {
  trail.pop();
  const prev = trail[trail.length - 1];
  if (prev) renderArticle(prev); else renderIndex();
}

export function codexSearch(q) {
  const out = document.getElementById('codex-results');
  if (!out) return;
  if (!q.trim()) { out.innerHTML = ''; return; }
  const hits = searchArticles(q, index, bodies);
  out.innerHTML = hits.length
    ? hits.map(a => `<a class="cx-link cx-result" href="#" data-cx="${a.id}">${esc(a.title)}</a>`).join('')
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
      items.map(a => `<a class="cx-link cx-entry" href="#" data-cx="${a.id}">${esc(a.title)}</a>`).join('') +
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

function renderArticle(id) {
  const a = index.articles.find(a => a.id === id);
  setHeader(a.title, true);
  const t = titles();
  const back = backlinksTo(id, bodies);
  const related = (a.related || []).filter(r => t[r]);
  const body = document.getElementById('codex-body');
  body.innerHTML =
    `<div class="cx-crumb">${esc(a.category)}</div>` +
    `<article class="cx-article">${renderMarkdown(bodies[id] || '', t)}</article>` +
    (related.length ? '<div class="cx-foot"><span class="cx-foot-label">See also</span> ' +
      related.map(r => `<a class="cx-link" href="#" data-cx="${r}">${esc(t[r])}</a>`).join(' · ') + '</div>' : '') +
    (back.length ? '<div class="cx-foot"><span class="cx-foot-label">Linked from</span> ' +
      back.map(r => `<a class="cx-link" href="#" data-cx="${r}">${esc(t[r])}</a>`).join(' · ') + '</div>' : '');
  body.scrollTop = 0;
}

// One delegated listener for every link the panel ever renders. Bound in
// initCodex, not at module top level: dispatch_window.js imports this module,
// and a top-level DOM touch would run in every module (and test) that
// imports the dispatch window — the fragility ws.js's header warns about.
function onPanelClick(e) {
  const a = e.target.closest && e.target.closest('#codex-panel [data-cx]');
  if (!a) return;
  e.preventDefault();
  openCodex(a.dataset.cx);
}
