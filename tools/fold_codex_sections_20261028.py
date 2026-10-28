#!/usr/bin/env python3
"""One-time: fold Codex fragment articles back into their topic as sections.

c87e750d split each topic into one article per heading (ids like
`colonies-founding-a-colony-2`). Most fragments were reachable only from
their parent's related list, so a hub such as "Types of ship" linked to
nothing. This folds every fragment into its root article as a `## Title {#old-id}`
section. The old id stays addressable: index.json lists it under the root's
`sections`, and links, notification kinds and `keryx codex <old-id>` resolve
it to the section. EXTRA_PARENTS folds small standalone articles into the
topic they belong to. Run once; history documents the move.
"""
from pathlib import Path
import json, re, sys

ROOT = Path(__file__).resolve().parents[1]
D = ROOT / 'web/static/codex'
index = json.loads((D / 'index.json').read_text())
A = {a['id']: a for a in index['articles']}
ids = set(A)

# Standalone articles that are a subsection of another topic (id → topic).
EXTRA_PARENTS = json.loads(sys.argv[1]) if len(sys.argv) > 1 else {}


def parent(i):
    if i in EXTRA_PARENTS:
        return EXTRA_PARENTS[i]
    m = re.match(r'^(.*)-(\d+)$', i)
    if m and m[1] in ids:
        return m[1]
    best = None
    for p in ids:
        if i != p and i.startswith(p + '-') and i in A[p].get('related', []):
            if not best or len(p) > len(best):
                best = p
    return best


children = {}
for i in sorted(ids):
    p = parent(i)
    if p:
        children.setdefault(p, []).append(i)


def ordered(p):
    rel = A[p].get('related', [])
    kids = children.get(p, [])
    return [r for r in rel if r in kids] + [k for k in kids if k not in rel]


def body(i):
    return (D / (i + '.md')).read_text().strip()


def fold(p, depth, sections):
    parts = [body(p)] if depth == 0 else []
    for k in ordered(p):
        a = A[k]
        level = '##' if depth == 0 else '###'
        parts.append(f"{level} {a['title']} {{#{k}}}\n\n{body(k)}")
        sec = {'id': k, 'title': a['title']}
        for key in ('kinds', 'aliases'):
            if a.get(key):
                sec[key] = a[key]
        sections.append(sec)
        sub = fold(k, depth + 1, sections)
        if sub:
            parts.extend(sub)
    return parts


roots = [a for a in index['articles'] if not parent(a['id'])]
folded = {i for i in ids if parent(i)}
out = []
for a in roots:
    sections = []
    parts = fold(a['id'], 0, sections)
    if sections:
        (D / (a['id'] + '.md')).write_text('\n\n'.join(parts) + '\n')
        a = dict(a)
        a['related'] = [r for r in a.get('related', []) if r not in folded or parent(r) != a['id']]
        a['sections'] = sections
        if not a['related']:
            del a['related']
    out.append(a)
for i in folded:
    (D / (i + '.md')).unlink()
index['articles'] = out
(D / 'index.json').write_text(json.dumps(index, ensure_ascii=False, indent=2) + '\n')
print(f'{len(roots)} articles, {len(folded)} folded into sections')
