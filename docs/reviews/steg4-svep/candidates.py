#!/usr/bin/env python3
"""Eight unranked ownership candidates with reproducible, separate metrics."""
import gzip
import json
import pathlib

HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parents[2]
A=json.loads(gzip.decompress((HERE/'raw/ast.json.gz').read_bytes()))
CHURN=json.loads((HERE/'classified/churn-60days.json').read_text())
SPECS=[
 dict(id='A',title='Rekrytering: konkret beslut och atomisk domänhandling',rows=[4,15,16],
      copies=['server/api/handlers/province.go:364','server/api/handlers/province.go:2005','server/internal/capabilities/province_verbs.go:77','server/internal/province/training.go:107'],
      copy_scope='3 beslutsställen + katalog (inte fyra identiska kloner)',
      symbols=['canRecruit','CanRecruit','populationRequirement','PopulationRequirement'],
      consumers='Konkret Recruit, status, capabilities; web + Keryx läser status och skickar samma request; Codex beskriver gates (4 ytor).',
      risk='Hög: falsk affordability, fel crew/count, delbetalning/job. Verkliga rättelser 28b930ab och d6436593; ingen slutsats att varje kvarvarande kopia redan gett en bugg.',
      bugs=['28b930ab','d6436593'],guard='G1 domänhem + AST-gräns för handler-DB; gemensamt Requirement/Decision-kontrakt med rollback/samtidighet. Referensslice steg 6.'),
 dict(id='B',title='Debitering på anroparens TX',rows=[7],
      copies=['server/api/handlers/helpers.go:156','server/internal/combat/standing_orders.go:784','server/api/handlers/province.go:3390','server/api/handlers/settlement.go:599','server/api/handlers/messenger.go:471'],
      copy_scope='2 generella helpers + 3 utvalda inline debitställen; hela SQL-inventeringen är större',
      symbols=['deductGoods','deductGood'],
      consumers='6 direkta helper-call-sites; dessutom inline barter/upkeep/offer debits. Build/Recruit/Repair/StandingOrder m.fl.; server + webb/Keryx + Codex ekonomiska villkor (4 ytor).',
      risk='Hög: partial debit, calc_tick och revalidering. d6436593 visar verklig rekryteringsatomikbugg i en konsument, inte bevis att två helperkroppar i sig orsakat den.',
      bugs=['d6436593'],guard='SQL/AST-vakt mot privata settled/debit-fragment i namngivna migrerade konsumenter; kontrakt på TX med multi-good avslag och samtidighet. G1 province-hem.'),
 dict(id='C',title='Kredit/refund med explicit cap- och spillpolicy',rows=[8,9],
      copies=['server/api/handlers/logistics.go:79','server/internal/economy/trade.go:285','server/internal/economy/trade_return.go:81','server/internal/transport/arrival.go:146','server/internal/economy/gift.go:115','server/api/handlers/province.go:1515','server/internal/combat/unit_arrival.go:448'],
      copy_scope='4 exakt normaliserade UPSERT-kopior + 3 olika credit/refund-policyer; de sista är INTE samma kontrakt',
      symbols=['HandleGift','cancelQueuedBuild'],
      consumers='7 utvalda mutationsvägar för delivery/return/transfer/gift/build/arrival; egna replay-/notiskontrakt. Alla 4 ytor observerar stock och spill.',
      risk='Hög konservationsrisk. Gåvans cap/spill/returned-prov finns i docs/reviews/gava, men ingen kvarvarande credit-dubblett har här belagts orsaka en historisk bugg. Spill och refund får inte tyst harmoniseras.',
      bugs=[],guard='Återanvänd storage-cap-vakten; ny SQL-kopievakt över migrerade UPSERT samt konservations-/replaykontrakt. province-adapter undviker transport→economy.'),
 dict(id='D',title='ProductionEvaluation: gemensam inhämtning, skilda vyer',rows=[12,13,19],
      copies=['server/internal/economy/placement_yield.go:182','server/internal/economy/placement_yield.go:899','server/internal/economy/recompute.go:687','server/internal/economy/recompute.go:786','server/internal/economy/founding_forecast.go:58','server/internal/economy/catchment.go:35'],
      copy_scope='3 vyfamiljer actual/fullcrew/founding; optionsägarna delas redan. 2 refining/bemanning-loopar dupl t50. CatchmentBasePotential är RTA-test-only.',
      symbols=['LoadHexProductionOptionsAt','LoadHexProductionOptions','LoadBuildingProductionOptions','FullCrewPotential','RecomputeProduction','FoundingGrainNetPerTick'],
      consumers='Placeringsval, faktisk recompute, catchment-preview, founding och byggnadseffekt; samtliga 4 ytor. Antal direkta statiska call-sites nedan avser nuvarande shared helpers, inte antal kopierade formler.',
      risk='Hög för brist→brons. d4ed8c36 rättade held_workers i options och 6b64cda3/ea53d693 ersatte handskrivna effektpåståenden; dessa visar vyparitetsrisk, inte att en total regelrewrite behövs.',
      bugs=['d4ed8c36','6b64cda3','ea53d693'],guard='AST-vakt mot pensionerade helpers först efter konsumtionsprov; actual/fullcrew/founding kontrakt på samma data med blockad/FOW/tagning. Hex/refining förblir skilda.'),
 dict(id='E',title='Hexavstånd: Go-kanon och SQL-adapter',rows=[17],
      copies=['server/internal/hexgrid/hexgrid.go:56','server/internal/province/hex.go:11','server/internal/religion/model.go:83','server/internal/world/mapgen.go:3765','server/cmd/keryx/cmd_map.go:13','server/internal/economy/siege.go:77','server/internal/combat/unit_intercept_scan.go:168','server/internal/transport/intercept.go:202','server/api/handlers/settlement.go:1450','server/internal/gossip/gossip.go:79'],
      copy_scope='5 Go-formelägare + inline SQL (10 ankare här; fler hex-SQL statements redovisas separat)',
      symbols=['Distance','HexDistance','hexDist'],
      consumers='Go geometry consumers enligt separata call-site/file-tal; SQL i siege/interception/reveal/gossip/arrival/join. Server + Keryx samt kart-/Codex-kontrakt, räckvidd på 4 ytor men ingen ny spelarmekanik.',
      risk='Medel per konsument, bred spridning. Ingen verifierad historisk hexavståndsbugg hittades i denna slice; negativa koordinater/range och dubbla representationsägare är risken.',
      bugs=[],guard='G1-klassificera religion/world→hexgrid innan flytt; SQL hex_distance kontraktsprov mot Go och text-SQL-vakt mot namngivna inline-formler.'),
 dict(id='F',title='Kontaktmängd: pensionera privat visibleOrigins',rows=[6],
      copies=['server/internal/capabilities/context.go:191','server/internal/province/contacts.go:9'],
      copy_scope='2 regler med exakt samma SQL efter kommentar-normalisering; kontrollfixture för mätningen',
      symbols=['VisibleOrigins','visibleOrigins'],
      consumers='3 direkta call-sites: handler world.loadVisibleOrigins, transfer capability och privat contacted-check. Brev/handel/gåva når web+Keryx; Codex beskriver kontakten (4 ytor).',
      risk='Hög informationsrisk vid drift, liten teknisk slice. Ingen belagd historisk FOW-bugg från just denna kvarvarande kopia. 31224eb7 visar angränsande verklig silverfilterbugg; SELLABLE≠SHIPPABLE ska bevaras, inte användas som argument att slå ihop katalogfilter.',
      bugs=[],guard='AST-förbud mot privata visibleOrigins efter migration till province; kontakt/live/minne fortsätter vara skilda begrepp, samma FOW-repro och mutation av privat återkopia.'),
 dict(id='G',title='Passage-/hamnpredikat och portval',rows=[10,18],
      copies=['server/api/handlers/messenger_passage.go:399','server/api/handlers/messenger_passage.go:412','server/internal/province/pathfind.go:157','server/api/handlers/messenger_passage.go:485','server/internal/messenger/passage.go:501','server/internal/combat/ship_hull.go:417','server/internal/transport/carrier.go:155','server/internal/economy/trade_return.go:195'],
      copy_scope='3 olika terrängpredikat (naval match har samma lista); 2 passage eligibility SQL-kopior; 3 nearest-port SQL-läsningar, 2 t100 loopkloner. Små separata ägarslicar, inte ett gemensamt IsPassable för allt.',
      symbols=['isDryLandTerrain','isSeaOrRiverTerrain','isPassable','IsPassable','NearestOwnPort','nearestOwnShipyardSettlement'],
      consumers='Passageval/server-preview, A*, reroute/shipyard/return/interception; påverkade utfall via 4 ytor. Direkta helper-call-sites nedan exkluderar inline SQL.',
      risk='Hög lifecycle-/destinationrisk. f867e596 rättade en verklig limped destination i samma port/returflöde, inte bevis att loopklonen gav felet. river_ford är navigerbart/gångbart men inte dry-land landing.',
      bugs=['f867e596'],guard='G1 province-policyprimitiver; predikatmatris plus SQL/AST-vakt över migrerade kopior. Bevara shipyard preferens, portens tillstånd, egen ägare och verklig rutt.'),
 dict(id='H',title='Sparad rörelse kontra legacy interpolation',rows=[10,11],
      copies=['server/internal/movement/movement.go:134','server/internal/province/interpolate.go:20','server/internal/province/eyes.go:415','server/internal/transport/intercept.go:503','server/internal/transport/transport.go:136','server/internal/economy/trade_return.go:130','server/internal/transport/arrival.go:96'],
      copy_scope='4 legacy positionsvägar jämfört med shared movement-kärna + 2 journey validation-kopior; CurrentPosition är RTA-test-only',
      symbols=['InterpolatePosition','InterpolateAlongPath','CurrentPosition','straightLineHexPosition','SavedPosition','RoutePositionAt'],
      consumers='FOW/loadLiveEyes, interception, unit/world-vyer och returfrigörande; server + webbkarta/Keryx + Codex position/reselogik (4 ytor). NULL-journey är explicit legacykontrakt.',
      risk='Hög men större substrate. 6e90d076 och 3911cbe3 rättade verkliga route/recall-fel. Det bevisar inte att alla legacyfallbacks kan rivas nu; programme steg10 väntar efter playtest.',
      bugs=['6e90d076','3911cbe3'],guard='AST-förbud mot gamla interpolatorer med namngiven krympande legacy-undantagslista; sparad rutt/entering-costs kontrakt. Ingen automatisk deletion av RTA-test-only.'),
]

records=[]
for spec in SPECS:
    anchors=[]
    for anchor in spec['copies']:
        file,line=anchor.rsplit(':',1)
        assert 1<=int(line)<=len((ROOT/file).read_text().splitlines()),anchor
        anchors.append(dict(file=file,line=int(line),source_line=(ROOT/file).read_text().splitlines()[int(line)-1]))
    files=sorted({a['file'] for a in anchors})
    churn={f:CHURN.get(f,dict(commits=[],commit_count=0,added=0,deleted=0,binary_changes=0)) for f in files}
    calls=[s for s in A['calls'] if s['value'] in spec['symbols'] and not s['file'].endswith('_test.go')]
    record=dict(**spec, anchor_count=len(anchors),anchor_source=anchors,source_files=files,
                churn_60days=dict(file_totals={f:{k:v for k,v in c.items() if k!='commits'} for f,c in churn.items()},
                                  distinct_commits=len({c for ch in churn.values() for c in ch['commits']}),
                                  added=sum(ch['added'] for ch in churn.values()),deleted=sum(ch['deleted'] for ch in churn.values())),
                call_sites=len(calls),consumer_files=len({s['file'] for s in calls}),
                calls=[{k:s[k] for k in ('file','line','function','value','receiver','package') if k in s} for s in calls])
    records.append(record)
(HERE/'classified/candidates.json').write_text(json.dumps(records,indent=2,ensure_ascii=False)+'\n')
md=['# Åtta kandidater — inte slutrangordnade','',
    'Bas `daf740dd`. Churn avser 60 dygn före basens committertid (exakt intervall i raw/provenance.json), i kandidatens angivna källfiler: distinct git commits samt tillagda/borttagna rader. Filens churn omfattar även annan kod och är inte funktionens churn. Rename behandlas som delete/add. Kopietal är semantiska ställen, aldrig summan av överlappande dupl/SQL-träffar. Call-sites är syntaktiska produktionsanrop till angivna namn; antal konsumentfiler är deduplicerat. Det är varken dynamisk frekvens eller en bevisad call-graph. Fyra ytor anger beteendets räckvidd, inte fyra duplicerade implementationsägare.','',
    '| ID | Kandidat/ägarrad | Ankare / kopior | 60d commits; +/− rader | Helper-anrop / konsumentfiler |','|---|---|---|---|---|']
for r in records:
    ch=r['churn_60days'];md.append(f"| {r['id']} | {r['title']} ({'/'.join(map(str,r['rows']))}) | {r['anchor_count']}; se avgränsning | {ch['distinct_commits']}; +{ch['added']}/−{ch['deleted']} | {r['call_sites']} / {r['consumer_files']} |")
for r in records:
    md+=['',f"## {r['id']}. {r['title']}",'',
         'Kopior: '+ ' · '.join('`'+x+'`' for x in r['copies'])+'.', '',
         r['copy_scope']+'.','',r['consumers'],'',r['risk'],'',
         'Möjlig vakt: '+r['guard'], '',
         'Mätta namn: '+', '.join('`'+x+'`' for x in r['symbols'])+'. Filvis churn och varje call-site finns i candidates.json.']
md+=['','Förslag till Claude: A passar den redan beslutade referensslicen; F är en liten tydlig regelflytt med bevisad kopia. B/C bör följa i kontrakterade konsumentsteg. D behöver paritetskontrakt före flytt. E kan vaktas tydligt men har ingen belagd aktuell bugghistoria här. G delas i predikat, passage-read och portval; H följer programmets väntan efter playtest. Detta är underlag, ingen slutlig prioritering eller implementation.']
(HERE/'classified/candidates.md').write_text('\n'.join(md)+'\n')
print('\n'.join(md[4:14]))
