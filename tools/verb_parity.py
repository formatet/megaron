#!/usr/bin/env python3
"""Read-only source parity inventory. Presence is static evidence, not runtime proof.

No dependencies beyond Python. Supported expressions: literals, templates,
concatenation, fmt.Sprintf, lexical variables and conditional alternatives.
Unresolved request paths/methods are reported separately, never counted present.
"""
import argparse
import fnmatch
import itertools
import hashlib
import json
from pathlib import Path
import re
from dataclasses import dataclass

ROOT = Path(__file__).resolve().parents[1]
METHODS = {'GET', 'POST', 'PUT', 'DELETE', 'PATCH'}

@dataclass
class Token:
    value: str
    kind: str
    line: int

# Strings are recognized before comments; URL // cannot become a comment.
LEX = re.compile(r'''(?P<string>"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`[^`]*`)|(?P<comment>//[^\n]*|/\*[\s\S]*?\*/)|(?P<regex>/(?:\\.|\[(?:\\.|[^\]\\])*\]|[^/\n])+/[gimsuy]*)|(?P<id>[A-Za-z_$][\w$]*)|(?P<op>:=|=>|==|!=|&&|\|\||.)''', re.S)

def lex(source):
    out, pos = [], 0
    while pos < len(source):
        m = LEX.match(source, pos)
        if not m: raise ValueError('cannot tokenize source')
        kind, value, end = m.lastgroup, m.group(), m.end()
        if kind == 'regex' and out and out[-1].value not in ('(', '=', ':=', ':', ',', 'return', '=>', '[', '?', '||', '&&', '!'):
            kind, value, end = 'op', '/', pos+1
        if kind != 'comment' and not value.isspace():
            out.append(Token(value, kind, source.count('\n', 0, pos) + 1))
        pos = end
    return out

def split_top(tokens, separator):
    parts, start, stack = [], 0, []
    for i, t in enumerate(tokens):
        if t.value in ('(', '[', '{'): stack.append(t.value)
        elif t.value in (')', ']', '}') and stack: stack.pop()
        elif t.value == separator and not stack:
            parts.append(tokens[start:i]); start = i + 1
    return parts + [tokens[start:]]

def matching(tokens, start):
    pairs = {'(': ')', '{': '}', '[': ']'}
    stack = []
    for i in range(start, len(tokens)):
        v = tokens[i].value
        if v in pairs: stack.append(pairs[v])
        elif stack and v == stack[-1]:
            stack.pop()
            if not stack: return i
    raise ValueError('unbalanced source at line ' + str(tokens[start].line))

def canonical(path):
    path = path.split('?', 1)[0].rstrip('/') or '/'
    path = re.sub(r'\{[^}]+\}|:[A-Za-z_]\w*|%[sdv]|<id>', '{id}', path)
    return path

def values(tokens, env):
    """Conservative finite set of source path shapes; unknown scalars are <id>."""
    if not tokens: return {'<id>'}
    if tokens[0].value == '(' and matching(tokens, 0) == len(tokens)-1:
        return values(tokens[1:-1], env)
    # Ternary arms are alternatives, not concatenated literals.
    cond = split_top(tokens, '?')
    if len(cond) == 2:
        arms = split_top(cond[1], ':')
        if len(arms) == 2: return values(arms[0], env) | values(arms[1], env)
    parts = split_top(tokens, '+')
    if len(parts) > 1:
        candidates = [values(p, env) for p in parts]
        if any(len(c) > 20 for c in candidates): return {'<id>'}
        return {''.join(p) for p in itertools.islice(itertools.product(*candidates), 100)}
    if len(tokens) == 1:
        t = tokens[0]
        if t.kind == 'id': return env.get(t.value, {'<id>'})
        if t.kind == 'string':
            s = t.value[1:-1]
            if t.value[0] == '`':
                def substitute(m):
                    resolved = values(lex(m.group(1)), env)
                    return next(iter(resolved)) if len(resolved) == 1 else '<id>'
                s = re.sub(r'\$\{([^}]+)\}', substitute, s)
            return {s}
    if len(tokens) > 4 and ''.join(t.value for t in tokens[:4]) == 'fmt.Sprintf(':
        end = matching(tokens, 3)
        args = split_top(tokens[4:end], ',')
        formats = values(args[0], env)
        result = set()
        for fmt in formats:
            holes = list(re.finditer(r'%[sdv]', fmt))
            choices = [values(arg, env) for arg in args[1:]]
            if len(holes) != len(choices): continue
            for combination in itertools.islice(itertools.product(*choices), 100):
                text, cursor = '', 0
                for hole, replacement in zip(holes, combination):
                    text += fmt[cursor:hole.start()] + replacement; cursor = hole.end()
                result.add(text + fmt[cursor:])
        return result or {'<id>'}
    return {'<id>'}

def expression_end(tokens, start):
    """Assignment RHS ends at a top-level newline or statement terminator."""
    stack = []
    for i in range(start, len(tokens)):
        v = tokens[i].value
        if not stack and (v in (';', '}') or (i > start and tokens[i].line > tokens[i-1].line and tokens[i-1].value not in ('+', '?', ':', ',', '=') and v not in ('?', ':', '+'))):
            return i
        if v in ('(', '[', '{'): stack.append(v)
        elif v in (')', ']', '}'):
            if not stack: return i
            stack.pop()
    return len(tokens)

def route_inventory(source, helper_prefixes=None):
    tokens, rows, scopes, bodies = lex(source), [], [''], {}
    helper_prefixes = helper_prefixes or {}
    for i, t in enumerate(tokens):
        v = t.value
        if v == 'func' and i+1 < len(tokens) and tokens[i+1].value in helper_prefixes:
            j = i+2
            while tokens[j].value != '{': j += 1
            bodies[j] = helper_prefixes[tokens[i+1].value]
        if v == 'Route' and tokens[i+1].value == '(':
            end = matching(tokens, i+1)
            args = split_top(tokens[i+2:end], ',')
            if len(args) >= 2 and args[0][0].kind == 'string':
                body = next(j for j in range(i+2, end) if tokens[j].value == '{')
                bodies[body] = scopes[-1] + next(iter(values(args[0], {})))
        if v == '{': scopes.append(bodies.get(i, scopes[-1]))
        elif v == '}': scopes.pop()
        elif v.upper() in METHODS and i > 0 and tokens[i-1].value == '.' and tokens[i+1].value == '(':
            args = split_top(tokens[i+2:matching(tokens, i+1)], ',')
            for path in values(args[0], {}):
                full = canonical(scopes[-1] + path)
                if full.startswith('/api/v1/'):
                    rows.append({'method': v.upper(), 'path': full, 'line': t.line})
    return rows

def function_parameters(source):
    tokens, definitions = lex(source), {}
    for i, t in enumerate(tokens[:-3]):
        if t.value != 'func' or tokens[i+1].kind != 'id' or tokens[i+2].value != '(': continue
        end = matching(tokens, i+2)
        params = []
        for group in split_top(tokens[i+3:end], ','):
            params.append(group[0].value if group else '')
        j = end+1
        while j < len(tokens) and tokens[j].value != '{': j += 1
        if j < len(tokens): definitions[tokens[i+1].value] = (params, j)
    return definitions

def calls(source, language, file='fixture', bindings=None, collect=None):
    tokens, found, unresolved, scopes = lex(source), [], [], [{}]
    bindings = bindings or {}
    definitions = function_parameters(source) if language == 'go' else {}
    seeded_bodies = {body: bindings.get(name, {}) for name, (_, body) in definitions.items()}
    # Injected URL object properties are finite input evidence (not invented routes).
    properties = {}
    if language == 'js':
        for j, tok in enumerate(tokens[:-2]):
            if tok.kind == 'id' and tokens[j+1].value == ':':
                candidate = values(split_top(tokens[j+2:expression_end(tokens, j+2)], ',')[0], {})
                candidate = {x for x in candidate if x.startswith('/api/v1/')}
                if candidate: properties.setdefault(tok.value, set()).update(candidate)
        scopes[0].update(properties)
    for i, t in enumerate(tokens):
        v = t.value
        env = {}
        for scope in scopes: env.update(scope)
        if t.kind == 'id' and i+1 < len(tokens) and tokens[i+1].value in (':=', '='):
            start = i+2
            scopes[-1][v] = values(tokens[start:expression_end(tokens, start)], env)
        # All block scopes are tracked; data literals do not alter outer bindings.
        if v == '{': scopes.append(seeded_bodies.get(i, {}).copy())
        elif v == '}' and len(scopes) > 1: scopes.pop()
        cli = language == 'go' and v in ('get', 'post', 'put', 'delete', 'patch', 'do') and i > 0 and tokens[i-1].value == '.'
        web = language == 'js' and (v in ('fetchAuth', 'fetch', 'fetchImpl', 'read') or (v == 'get' and (i == 0 or tokens[i-1].value != '.')))
        if collect is not None and t.kind == 'id' and i+1 < len(tokens) and tokens[i+1].value == '(' and not (i > 0 and tokens[i-1].value in ('func', '.')):
            arguments = split_top(tokens[i+2:matching(tokens, i+1)], ',')
            collect.append((v, [values(a, env) for a in arguments]))
        if not (cli or web) or i+1 >= len(tokens) or tokens[i+1].value != '(': continue
        # Exclude function declarations and method definitions.
        if i > 0 and tokens[i-1].value == 'function': continue
        end = matching(tokens, i+1)
        args = split_top(tokens[i+2:end], ',')
        if cli and v == 'do':
            if len(args) < 2: continue
            method_expr = ''.join(x.value for x in args[0])
            methods = values(args[0], env)
            if method_expr.startswith('http.Method'): methods = {method_expr[len('http.Method'):].upper()}
            paths = values(args[1], env)
        else:
            paths = values(args[0], env)
            methods = {v.upper()} if cli else {'GET'}
            if web and len(args) > 1:
                opts = args[1]
                for j, tok in enumerate(opts[:-1]):
                    if tok.value == 'method' and opts[j+1].value in (',', '}'):
                        methods = env.get('method', {'<id>'})
                    if tok.value.strip('\'"') == 'method' and opts[j+1].value == ':':
                        stop = j+2
                        while stop < len(opts) and opts[stop].value not in (',', '}'): stop += 1
                        methods = values(opts[j+2:stop], env)
        known = False
        for path in sorted(paths):
            # BASE is an unknown origin, not a route segment.
            path = re.sub(r'^<id>(?=/api/v1/)', '', path)
            if not path.startswith('/api/v1/'): continue
            for method in methods:
                if method not in METHODS: continue
                found.append({'method': method, 'path': canonical(path), 'file': file, 'line': t.line})
                known = True
        if not known and (v != 'get' or cli):
            # Generic transport internals are also visible here, rather than silently guessed.
            unresolved.append({'file': file, 'line': t.line, 'call': v, 'paths': sorted(paths), 'methods': sorted(methods)})
    return found, unresolved

def documentation(routes, articles, aliases):
    result = {}
    for route in routes:
        key = route['method'] + ' ' + route['path']
        # Curated semantic aliases are evidence of a mention, not endpoint coverage.
        rule = aliases.get(route['path'], {})
        terms = rule.get('terms', [])
        evidence = []
        for file, text in articles.items():
            if rule and Path(file).name not in rule.get('articles', []): continue
            for line, content in enumerate(text.splitlines(), 1):
                exact = route['path'] in content
                mention = any(re.search(r'(?<![\w-])' + re.escape(term) + r'(?![\w-])', content, re.I) for term in terms)
                if exact or mention:
                    evidence.append(f'{file}:{line}'); break
        result[key] = evidence
    return result

def exclusions(route, allowlist):
    key = route['method'] + ' ' + route['path']
    return [row['reason'] for row in allowlist if fnmatch.fnmatchcase(key, row['route'])]

def verblist_routes(text):
    found = set()
    for match in re.finditer(r'(GET|POST|PUT|DELETE|PATCH)(?:/(GET|POST|PUT|DELETE|PATCH))*\s+(/[^`\s,|]+)', text):
        methods = re.findall(r'GET|POST|PUT|DELETE|PATCH', match.group().split(' ', 1)[0])
        path = match.group(3)
        # Optional :id segment expands into collection and item routes.
        optional = re.search(r'\[(/:[A-Za-z_]\w*)\]', path)
        variants = [path[:optional.start()]+path[optional.end():], path[:optional.start()]+optional.group(1)+path[optional.end():]] if optional else [path]
        for variant in variants:
            if variant.startswith('/worlds/'): full = '/api/v1' + variant
            elif variant.startswith(('/auth/', '/notification-preferences')): full = '/api/v1' + variant
            else: full = '/api/v1/worlds/{id}' + variant
            for method in methods:
                if optional and ((method in ('GET','POST')) == (variant == variants[1])): continue
                found.add(method + ' ' + canonical(full))
    # Backticked leaf paths inherit the preceding method/path context in the
    # same table cell: load,/unload and standing-orders[/:id],/pause,/resume.
    for line in text.splitlines():
        if not line.startswith('|'): continue
        cells = line.split('|')
        if len(cells) < 4: continue
        previous, method = None, None
        for fragment in re.findall(r'`([^`]+)`', cells[2]):
            explicit = re.match(r'(GET|POST|PUT|DELETE|PATCH)(?:/(GET|POST|PUT|DELETE|PATCH))*\s+(/[^\s]+)', fragment)
            if explicit:
                method = explicit.group(1)
                previous = explicit.group(3)
            elif fragment.startswith('/') and previous and method:
                path = fragment
                if fragment.count('/') == 1:
                    if '[' in previous:
                        parent = re.sub(r'\[([^]]+)\]', r'\1', previous)
                    else:
                        parent = previous.rsplit('/', 1)[0]
                    path = parent + fragment
                if path.startswith('/worlds/'): full = '/api/v1'+path
                elif path.startswith(('/auth/', '/notification-preferences')): full = '/api/v1'+path
                else: full = '/api/v1/worlds/{id}'+path
                found.add(method+' '+canonical(full))
    return found

def inventory(root, config, verblista):
    main = root / 'server/cmd/server/main.go'
    routes = route_inventory(main.read_text(), config['helper_prefixes'])
    seen, unknown = {'keryx': {}, 'webb': {}}, []
    go_sources = {f: f.read_text() for f in sorted((root/'server/cmd/keryx').glob('*.go')) if not f.name.endswith('_test.go')}
    definitions = {}
    for source in go_sources.values(): definitions.update(function_parameters(source))
    bindings = {}
    for _ in range(3):
        invoked = []
        for source in go_sources.values(): calls(source, 'go', bindings=bindings, collect=invoked)
        for name, arguments in invoked:
            if name not in definitions: continue
            for param, candidates in zip(definitions[name][0], arguments):
                concrete = {x for x in candidates if x != '<id>'}
                if concrete: bindings.setdefault(name, {}).setdefault(param, set()).update(concrete)
    for surface, directory, suffix, language in [
        ('keryx', root/'server/cmd/keryx', '.go', 'go'),
        ('webb', root/'web/static/js/megaron', '.js', 'js')]:
        for file in sorted(directory.rglob('*'+suffix)):
            if file.name.endswith(('_test.go', '.test.js', '.test.mjs')): continue
            rows, issues = calls(file.read_text(), language, str(file.relative_to(root)), bindings if language == 'go' else None)
            unknown.extend(issues)
            for row in rows:
                key = row['method']+' '+row['path']
                seen[surface].setdefault(key, []).append(f"{row['file']}:{row['line']}")
    articles = {str(f.relative_to(root)): f.read_text() for f in sorted((root/'web/static/codex').glob('*.md'))}
    docs = documentation(routes, articles, config['codex_aliases'])
    for route in routes:
        key = route['method']+' '+route['path']
        route['keryx'] = seen['keryx'].get(key, [])
        route['webb'] = seen['webb'].get(key, [])
        route['codex'] = docs[key]
        route['excluded'] = exclusions(route, config['allowlist'])
    listed = verblist_routes(verblista)
    all_registered = {r['method']+' '+r['path'] for r in routes}
    # The table uses trade-* and collection/item shorthand. Resolve these only
    # to existing registrations; never invent an endpoint from prose.
    expanded = set()
    for spec in listed:
        matches = {key for key in all_registered if fnmatch.fnmatchcase(key, spec)}
        method, path = spec.split(' ', 1)
        if not matches and method == 'DELETE' and spec+'/{id}' in all_registered:
            matches = {spec+'/{id}'}
        if not matches and method == 'POST' and path.endswith('/{id}') and method+' '+path[:-5] in all_registered:
            matches = {method+' '+path[:-5]}
        expanded.update(matches or {spec})
    listed = expanded
    actual = {r['method']+' '+r['path'] for r in routes if not r['excluded']}
    internal, pending = [], []
    for issue in unknown:
        reasons = [rule['reason'] for rule in config.get('unresolved_allowlist', []) if rule['file']==issue['file'] and rule['call']==issue['call']]
        if reasons: internal.append(dict(issue, reasons=reasons))
        else: pending.append(issue)
    fingerprint = hashlib.sha256()
    paths = [main, *go_sources, *sorted((root/'web/static/js/megaron').rglob('*.js')), *sorted((root/'web/static/codex').glob('*.md'))]
    for file in sorted(paths):
        fingerprint.update(str(file.relative_to(root)).encode()+b'\0'+file.read_bytes())
    fingerprint.update(verblista.encode())
    fingerprint.update(json.dumps(config,sort_keys=True).encode())
    return {'source_sha256': fingerprint.hexdigest(), 'routes': routes, 'unresolved': pending, 'transport_or_assets': internal, 'listed_not_registered': sorted(listed - {r['method']+' '+r['path'] for r in routes}),
            'registered_not_listed': sorted(actual - listed),
            'client_not_registered': {s: sorted(set(v)-{r['method']+' '+r['path'] for r in routes}) for s,v in seen.items()}}

def markdown(report):
    out = ['# Statisk verbparitet', '', 'Input SHA256: `'+report['source_sha256']+'`', '', '✓ = källanrop; Codex ✓ = uttryckligt rutt-/aliasomnämnande, inte fullständigt beteendebevis.',
           '— = ingen statisk evidens hittad; kontrollera olösta anrop före en verklig lucka hävdas.', '',
           '| Serverrutt | Keryx | Webb | Codex |', '|---|---|---|---|']
    for r in report['routes']:
        if r['excluded']: continue
        cells = []
        for surface in ('keryx','webb','codex'):
            evidence = r[surface]
            cells.append('✓ '+evidence[0] if evidence else '—')
        out.append('| '+r['method']+' '+r['path']+' | '+' | '.join(cells)+' |')
    for surface in ('keryx','webb','codex'):
        out += ['', '## Saknar statisk evidens: '+surface, '']
        out += ['- '+r['method']+' '+r['path'] for r in report['routes'] if not r['excluded'] and not r[surface]] or ['Inga.']
    out += ['', '## Dokumenterade undantag', '']
    out += ['- '+r['method']+' '+r['path']+': '+'; '.join(r['excluded']) for r in report['routes'] if r['excluded']]
    for label, key in [('I verblistan, inte registrerad','listed_not_registered'),('Registrerad, inget uttryckligt ruttmönster i verblistan','registered_not_listed')]:
        out += ['', '## '+label, ''] + (['- '+x for x in report[key]] or ['Inga.'])
    out += ['', '## Klientanrop utan registrerad rutt (även dynamiska kandidater)', '']
    for surface, keys in report['client_not_registered'].items(): out += ['- '+surface+': '+key for key in keys]
    out += ['', '## Olösta anrop (parserbegränsning, inte ytlucka)', '']
    out += ['- '+json.dumps(x, ensure_ascii=False) for x in report['unresolved']]
    out += ['', '## Generisk transport/statiska assets (dokumenterade undantag)', '']
    out += ['- '+json.dumps(x, ensure_ascii=False) for x in report['transport_or_assets']]
    return '\n'.join(out)+'\n'

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root', type=Path, default=ROOT)
    p.add_argument('--verblista', type=Path, default=Path.home()/'Dokument/myltavault/megaron_verblista.md')
    p.add_argument('--json', action='store_true')
    args = p.parse_args()
    config = json.loads((args.root/'tools/verb_parity_allowlist.json').read_text())
    if not args.verblista.is_file(): p.error('verblistan missing; pass --verblista (never silently omit comparison)')
    report = inventory(args.root, config, args.verblista.read_text())
    print(json.dumps(report, ensure_ascii=False, indent=2) if args.json else markdown(report), end='\n' if args.json else '')

if __name__ == '__main__': main()
