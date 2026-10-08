#!/usr/bin/env python3
"""Classify the pinned inventory, never rewrite measured production sources."""
import collections
import hashlib
import json
import gzip
import pathlib
import re

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
RAW = HERE / 'raw'
OUT = HERE / 'classified'
OUT.mkdir(exist_ok=True)
A = json.loads(gzip.decompress((RAW / 'ast.json.gz').read_bytes()))

def test(s):
    return s['file'].endswith('_test.go') or '.test.' in s['file']

def sql_normalize(v):
    # Do not eat -- inside quoted SQL strings; inventory literals here have no
    # such strings. Reject misleading prose emitted by the broad AST collector.
    v = re.sub(r"'(?:''|[^'])*'|--[^\n]*", lambda m: '' if m.group().startswith('--') else m.group(), v)
    v = ' '.join(v.lower().split())
    if v in ('update failed', 'delete failed', 'select'):
        return None
    return v

def owner(s):
    """Routing hints only. Explicit review overrides for actual clone findings."""
    f, v = s['file'], (s.get('value', '') + ' ' + s.get('function', '')).lower()
    if 'select distinct pos.q' in v or 'visibleorigins' in v: return [6]
    if 'hex_distance' in v or ('abs(' in v and ('map_q' in v or ' q' in v)): return [17]
    if 'settlement_goods' in v:
        if re.search(r'amount\s*=.*?\+\s*\$', v) or 'on conflict (settlement_id, good_key)' in v: return [8, 9]
        if re.search(r'amount\s*=.*?-\s*\$', v): return [7]
        return [7, 8, 9]
    if 'processed_' in v: return [3]
    if 'scheduled_events' in v: return [2]
    if 'current_tick' in v or 'current_world_tick' in v: return [1]
    if 'settlement_placement' in v: return [12, 13, 16]
    if 'population' in v: return [16]
    if 'production_rules' in v or 'recipes' in v: return [13, 19]
    if 'terrain' in v: return [18]
    if 'march_route' in v or 'journey' in v: return [10, 11]
    if 'units' in v or 'unit' in f: return [16]
    if 'owner_id' in v or 'province_id' in v or 'wanax_name' in v: return [5]
    return []

def dump(name, data):
    text=json.dumps(data, indent=2, ensure_ascii=False)+'\n'
    if name=='repeated-literals':
        (OUT/(name+'.json.gz')).write_bytes(gzip.compress(text.encode(),mtime=0))
    else:
        (OUT/(name+'.json')).write_text(text)

# Human-reviewed anchors are keys rather than unstable detector output order.
REVIEWS=json.loads((HERE/'clone-reviews.json').read_text())

def pairs(name):
    seen = set()
    for line in (RAW / (name+'.stdout')).read_text().splitlines():
        left, right = line.split(': duplicate of ')
        key = tuple(sorted((left, right)))
        if key not in seen:
            seen.add(key)
            yield key

def anchor(x):
    m = re.fullmatch(r'(.*):(\d+)-(\d+)', x)
    f, lo, hi = m[1], int(m[2]), int(m[3])
    fn = next((s['value'] for s in A['functions'] if s['file']==f and s['line']<=lo<=s['end']), '')
    return dict(file=f, line=lo, end=hi, function=fn)

clones = []
prod_keys = {}
for i, pair in enumerate(sorted(pairs('dupl-production-50')),1):
    review=REVIEWS[' | '.join(pair)]
    rows,kind,why=review['owner_rows'],review['classification'],review['reason']
    record = dict(id=f'GO{i:03}', sites=[anchor(x) for x in pair], owner_rows=rows,
                  classification=kind, reason=why, instrument='dupl-production-50')
    clones.append(record)
    prod_keys[pair] = record
assert len(prod_keys)==len(REVIEWS)==83
for pair in pairs('dupl-all'):
    if pair in prod_keys: continue
    sites = [anchor(x) for x in pair]
    is_test = any(test(s) for s in sites)
    assert is_test, 'Unexpected production pair: review explicitly'
    clones.append(dict(id=f'GO{len(clones)+1:03}', sites=sites,
                       owner_rows=[], classification='brus',
                       reason='Testfixture/helper-kopia; ingen andra produktionsregel. Test↔produktion kräver kontraktsprov, inte automatisk rivning.',
                       instrument='dupl-all-100', production_test_mixed=not all(test(s) for s in sites)))
dump('go-clones',clones)

js = json.loads((RAW/'jscpd/jscpd-report.json').read_text())
js_records=[]
for i,c in enumerate(js['duplicates'],1):
    sites=[dict(file='web/static/js/'+c[k]['name'],line=c[k]['start'],end=c[k]['end']) for k in ('firstFile','secondFile')]
    is_test=any(test(s) for s in sites)
    js_records.append(dict(id=f'JS{i:03}', sites=sites, owner_rows=[],
                           classification='brus' if is_test else 'avsiktlig parallellitet',
                           reason='DOM/mock bootstrap i testfixture' if is_test else 'Ingen kartrad: canvas clipping/raster/trädskugga återkommer för olika terränger; visuell familj, ingen serverregel',
                           tokens=c['tokens']))
dump('js-clones',js_records)

sqlgroups=collections.defaultdict(list)
fragments=collections.defaultdict(list)
rejected=[]
for s in A['sql']:
    if test(s):continue
    normalized=sql_normalize(s['value'])
    if normalized is None:
        rejected.append(s);continue
    sqlgroups[normalized].append(s)
    for line in s['value'].splitlines():
        line=sql_normalize(line)
        if line and len(line)>=60 and len(line.split())>=10:
            fragments[line].append(s)
records=[]
for normalized,sites in sqlgroups.items():
    if len(sites)<2:continue
    rows=owner(dict(file=sites[0]['file'],value=normalized))
    # Identical query is a representation/read/write primitive finding, not
    # proof that its callers have identical policies or TX ownership.
    kind='avsiktlig parallellitet' if normalized.startswith('select current') or 'processed_' in normalized else 'kandidat'
    records.append(dict(id=f'SQL{len(records)+1:03}', normalized=normalized,
                        sites=[{k:s[k] for k in ('file','line','function')} for s in sites],
                        owner_rows=rows, classification=kind,
                        reason='Samma normaliserade statement; rad anger berört kontrakt, inte order att slå ihop domänhandlingar' if kind=='kandidat' else 'Gemensam befintlig tid/claim-primitiv används av olika handlingar'))
dump('sql-statements',records)
all_sql=collections.defaultdict(list)
for site in A['sql']:
    norm=sql_normalize(site['value'])
    if norm is not None: all_sql[norm].append(site)
cross=[]
for norm,sites in all_sql.items():
    if any(test(s) for s in sites) and any(not test(s) for s in sites):
        cross.append(dict(normalized=norm,sites=[{k:s[k] for k in ('file','line','function') if k in s} for s in sites],
                          owner_rows=owner(dict(file=next(s['file'] for s in sites if not test(s)),value=norm)),
                          classification='brus/kontraktsfixture',reason='Samma SQL i produktionsväg och seed/assertion; ingen ytterligare produktionsägare, kontrollera att testet inte bara speglar fel formel'))
dump('sql-production-test',cross)
frag_records=[]
for normalized,sites in fragments.items():
    sites=list({(s['file'],s['line']):s for s in sites}.values())
    if len({s['file'] for s in sites})<2:continue
    frag_records.append(dict(normalized=normalized, sites=[{k:s[k] for k in ('file','line','function')} for s in sites],
                             owner_rows=owner(dict(file=sites[0]['file'],value=normalized)),
                             classification='fragment', reason='Gemensam SQL-rad; överlappar statementfynd, ej fristående kopietal'))
dump('sql-fragments',frag_records)
hexsql=[dict(file=s['file'],line=s['line'],function=s.get('function',''),value=s['value'],owner_rows=[17],classification='kandidat',reason='Minst tre ABS-uttryck i samma SQL-literal: axial avståndsformel (sum/2 eller max); inte bevis att övriga WHERE-policyer är identiska') for s in A['sql'] if not test(s) and len(re.findall(r'\babs\s*\(',s['value'],re.I))>=3]
dump('hex-sql',hexsql)
dump('sql-rejected-prose',rejected)

dc={}
for arm in ('production','with-tests'):
    funcs=[]
    for pkg in json.loads((RAW/f'deadcode-{arm}.stdout').read_text()):
        for f in pkg['Funcs']:
            funcs.append(dict(file='server/'+f['Position']['File'],line=f['Position']['Line'],value=f['Name']))
    dc[arm]=funcs
with_tests={(s['file'],s['value']) for s in dc['with-tests']}
DEAD_ROWS = {
 'loadLaborCapacities':[13,19], 'insufficientUnitsMsg':[15], 'ProvinceHandler.Marches':[10,11],
 'VoteWeighting':[], 'PopulationRequirement':[15,16], 'NewTestClock':[1], 'TestClock.Now':[1], 'TestClock.Set':[1], 'TestClock.Advance':[1],
 'CatchmentBasePotential':[13], 'rankedFoodSlotsAt':[13], 'GoodState.Current':[1,7,8], 'effectiveRates':[13], 'LaborRates':[13,16],
 'Worker.RegisterWithTimeout':[2], 'templeDevotionCapacity':[], 'templeTierMultiplier':[], 'LoyaltyFromPoints':[], 'clampPoints':[],
 'MessengerTravelDuration':[1,10], 'MessengerTravelTicks':[1,10], 'Presence':[11], 'FirstContact':[11],
 'OfferingShortfall':[], 'goodDivisor':[], 'SmoothDivineValue':[], 'CountsAsHolder':[], 'RevoltConditionsMet':[], 'LoyaltyProjection':[],
 'ticksDue':[1,2], 'CurrentPosition':[11], 'CombatCapable':[5,16], 'CanFoundMetropolis':[5,16],
 'RunnerName':[5], 'CaravanName':[5], 'journey':[5], 'possessive':[5], 'SpawnOreCatchmentScore':[12,13],
 'Client.patch':[], 'die':[]}
assert set(DEAD_ROWS)=={s['value'] for s in dc['production']}
dead=[]
for s in dc['production']:
    still=(s['file'],s['value']) in with_tests
    dead.append(dict(**s, owner_rows=DEAD_ROWS[s['value']], classification='ej nåbar enligt RTA' if still else 'test-only enligt RTA',
                     reason='Statisk nåbarhet från Go main, ej säker raderingsorder; runtime/plugin/reflektion och framtida användning bedöms separat'))
for s in dc['with-tests']:
    if test(s):dead.append(dict(**s,owner_rows=[],classification='brus',reason='Test-double metod ej nåbar ens med test; inget produktionsfynd'))
dump('deadcode',dead)

UNUSED_ROWS = {
 'dispatchShipReturnLeg':[10], 'placementYield':[13], 'ValidHexesForBuilding':[12,13], 'MarginalYieldForSlot':[13], 'goodCap':[9],
 'NormaliseCulture':[5], 'canBuild':[15], 'canCancelBuild':[15], 'canAllocate':[16], 'collapseSettlement':[4,16],
 'dispatchPlunderCaravan':[10,18], '*PassageScanHandler.releasePassageWaitOne':[10], '*PassageScanHandler.boardTransports':[10],
 '*SettlementHandler.applyOracleRevealDeposits':[6], '*BattleTickHandler.notifyBattleEnded':[3]}
unused=[]
for s in A['unused_parameters']:
    unused.append(dict(**s,owner_rows=[] if test(s) else UNUSED_ROWS.get(s['function'],[]),classification='brus' if test(s) else 'oanvänd parameter',
                       reason='Test-double signatur' if test(s) else 'Ingen identitetsbunden AST-användning i kroppen; adapter/signatur kan avsiktligt kräva parametern'))
dump('unused-parameters',unused)

literals=collections.defaultdict(list)
for s in A['literals']:
    if s['kind']=='STRING' and sql_normalize(s['value']) in sqlgroups:continue
    literals[(s['kind'],s['value'])].append(s)
literal_records=[]
for (kind,value),sites in literals.items():
    if len(sites)<2:continue
    prod=[s for s in sites if not test(s)]
    # Same number/string is not sufficient evidence of the same rule.
    rows=[]
    if value.replace('_','') in ('1000000','1000000.0'): rows=[9]
    elif value in ('0.005','166.67'):rows=[14]
    elif value in ('coastal_sea','deep_sea','river','river_ford','plains','mountains'):rows=[18]
    elif value in ('infantry','elite_infantry','chariot','ship','merchantman','galley','runner','caravan'):rows=[5,16,19]
    elif value in ('grain','fish','silver','bronze','timber','livestock','oil','wine'):rows=[19]
    literal_records.append(dict(kind=kind,value=value,production_count=len(prod),test_count=len(sites)-len(prod),
                                owner_rows=rows, classification='kontraktsnyckel/literal att bevaka' if rows else 'brus/ingen styrkt regelkopia',
                                reason=('CLI och economy speglar matkonstanter; andra 0.005-ställen är growth/starvation/tolerans och ska inte slås ihop' if value in ('0.005','166.67') else 'Samma värde är inte samma semantik; inga privata balansregler antas från numerisk likhet'),
                                sites=[{k:s[k] for k in ('file','line','function') if k in s} for s in sites]))
dump('repeated-literals',literal_records)

churn=collections.defaultdict(lambda:dict(commits=set(),added=0,deleted=0,binary_changes=0))
commit=None
for line in (RAW/'churn-60days.stdout').read_text().splitlines():
    if line.startswith('commit '):commit=line.split()[1];continue
    cols=line.split('\t')
    if len(cols)!=3:continue
    plus,minus,f=cols;record=churn[f];record['commits'].add(commit)
    if plus=='-':record['binary_changes']+=1
    else:record['added']+=int(plus);record['deleted']+=int(minus)
dump('churn-60days',{f:{**v,'commits':sorted(v['commits']),'commit_count':len(v['commits'])} for f,v in sorted(churn.items())})

db=collections.defaultdict(collections.Counter)
for s in A['db_calls']:db[s['file']][s['value']]+=1
all_files=sorted(p.as_posix() for p in (ROOT/'server/api/handlers').glob('*.go') if not p.name.endswith('_test.go'))
table=[]
for absolute in all_files:
    f=str(pathlib.Path(absolute).relative_to(ROOT)); counts=db[f]
    table.append(dict(file=f,total=sum(counts.values()),calls=dict(sorted(counts.items()))))
dump('handler-db-baseline',dict(methods=['Exec','Query','QueryRow','Begin','BeginTx','SendBatch','CopyFrom'],
                              receivers=['h.pool','pool','tx','db'],files=table,total=sum(x['total'] for x in table)))
md=['# DB-anrop per handlerfil — daf740dd','',
    'AST-call-sites i produktionsfiler: Exec, Query, QueryRow, Begin, BeginTx, SendBatch, CopyFrom på h.pool/pool/tx/db. Scan/Commit/Rollback/Close räknas inte; r.URL.Query är uttryckligen utesluten. Även hjälpfunktioner i handlerpaketet ingår. Måttet räknar kodställen, inte frågor per request eller kostnad. Alla 35 filer, även nollrader, ingår.', '',
    '| Fil | Totalt | QueryRow | Query | Exec | Begin/BeginTx | SendBatch/CopyFrom |','|---|---:|---:|---:|---:|---:|---:|']
for r in table:
    c=r['calls'];md.append(f"| {r['file']} | {r['total']} | {c.get('QueryRow',0)} | {c.get('Query',0)} | {c.get('Exec',0)} | {c.get('Begin',0)+c.get('BeginTx',0)} | {c.get('SendBatch',0)+c.get('CopyFrom',0)} |")
(OUT/'handler-db-baseline.md').write_text('\n'.join(md)+'\n')
summary=dict(go_pairs=len(clones),go_production_pairs=len(prod_keys),go_test_additions=len(clones)-len(prod_keys),
             mixed_production_test_pairs=sum(c.get('production_test_mixed',False) for c in clones),
             js_pairs=len(js_records),js_production_pairs=sum(not any(test(s) for s in c['sites']) for c in js_records),
             sql_production_test_groups=len(cross),sql_statement_groups=len(records),hex_sql_literals=len(hexsql),sql_fragment_groups=len(frag_records),sql_false_positive_literals=len(rejected),
             deadcode_production=len(dc['production']),deadcode_with_tests=len(dc['with-tests']),
             unused_parameters=len(unused),unused_production_parameters=sum(not test(s) for s in unused),
             literal_groups=len(literal_records),handler_files=len(table),db_calls=sum(r['total'] for r in table))
dump('summary',summary)
print(json.dumps(summary,indent=2))

# A browsable catalogue sits beside the complete machine-readable groups.
md=['# Klassificerade fynd — daf740dd','',
    'Varje JSON-post har owner_rows; [] betyder **ingen rad** i kartans 1–19. Kartraden anger berörd domänfråga, inte godkännande av en regelflytt. SQL-routing är innehållsbaserad (tabell/primitiv), inte en uppmätt transaktionsägare. Endast Go-klonernas omdömen är individuella manuella ankare i clone-reviews.json. SQL/literal/testfynd har reproducerbara mönsterklasser; de blir inte automatiskt backlogposter.','']
for title,items in [('Go kloner',clones),('JS kloner',js_records),('SQL statements',records),('Deadcode',dead),('Oanvända parametrar',unused)]:
    md += ['## '+title,'','| Ankare | Kartrad | Klass | Skäl |','|---|---|---|---|']
    for item in items:
        sites=item.get('sites',[item])
        anchors=' · '.join(f"`{s['file']}:{s['line']}`" for s in sites)
        rows='/'.join(map(str,item['owner_rows'])) or 'ingen rad'
        label=item['classification'].replace('|','/')
        reason=item['reason'].replace('|','/')
        if 'normalized' in item:reason='`'+item['normalized'][:100].replace('|','/').replace('`','')+'…` — '+reason
        if 'value' in item:reason='`'+item['value']+'` — '+reason
        md.append(f'| {anchors} | {rows} | {label} | {reason} |')
    md.append('')
md+=['## Literaler och testkopior','',
     '3150 upprepade Go-literalgrupper har exakta sites, production/test-count och mönsterklass i repeated-literals.json.gz. Tal är lexikala (1_000_000 och 1000000 hålls isär), inte ekvivalensbevis. `0.005` är i två fall matkonstant (economy/recompute.go:290 och cmd/keryx/cmd_status.go:94), men också tillväxt, svält och loggtolerans. `166.67` speglas som livestockFoodValue i economy/recompute.go:354 och cmd/keryx/cmd_status.go:101 (rad14). CLI-test cmd_status_surplus_test.go:118/125 använder CLI:s egna konstanter; det binder inte denna spegel till serverns auktoritativa värde. Detta är ett separat konsumentkontrakt att ta med i steg5, inte anledning att beställa growth-pin igen.', '',
     '43 normaliserade SQL-grupper korsar produktion/test (sql-production-test.json): seed och assertion delar statement, men är inte en ny produktionsregelägare. 27 ytterligare Go-klonpar vid t100 är test/test; inga production/test-par hittas vid den tröskeln. Ingen frånvaro av korta/semantiska testkopior påstås. 15 JS-par är DOM-mock bootstrap; de 3 produktionsparen är clipping/raster/skugga för skilda terränger och saknar kartrad.', '',
     '69 SQL-fragmentgrupper och 11 SQL-literals med ≥3 ABS uttryck är fullständigt listade separat. Överlappande fragment räknas inte som fler regelkopior.']
(OUT/'findings.md').write_text('\n'.join(md)+'\n')
