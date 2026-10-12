# Religion-alfa — inventering, implementation och verifiering

## Checkpoint B — granskningspunkt

Bas **2ddee968** (A mergad och LIVE av Claude), samma gren `codex/religion-alfa`.
B-grund **3858cfab**, separat receptcommit **242329a8**, migration **176**. Fem minoiska böner är aktiva.

**Timothys receptbeslut19:33 ingår i senaste B-hashen:**

| Bön | Grain | Timber | Fish | Silver |
|---|---:|---:|---:|---:|
| Vision | 10 | — | — | 100 |
| Walls | 8 | 4 | — | — |
| Green Shoots | 15 | — | — | 50 |
| Thread | — | 4 | — | 100 |
| Passage | — | — | 6 | 100 |

Produktionsdiffen i receptcommiten är endast dessa fem Offering-maps + Codex `rites`; testfixturer/prov följer med. Silvergolv100, chans/gates, väntetid och effekter är oförändrade. Web/Keryx använder serverkatalogen.

- Asterion: lokal ägarstämplad engångsgåva för nästa utresande landkaravan eller Runner. Tid ×0,75; landkaravanens överfallsradie2→1, Runner fortsatt helig. Arméer läser/förbrukar ingen av gåvorna.
- Stillhet: **The Sea Daimons of Stillness**, bön **Favourable Passage**. Nästa verkliga skepp från den egna staden får utresans tid ×0,75. Stormskadan läser en ägarstämpel för exakt avfärds-/ankomsttick. Bud ombord följer bärarens redan sparade resa. Stormväder, heltick-sampling och ordinarie skadebelopp orörda.
- `departure_prayers` konsumeras i samma TX som avfärden; pending kan inte förnyas, överförs inte till ny ägare och återställs vid rollback. Gudsväntan12, kharis-gates och successful-only från A. Förbrukning flyttar aldrig en pågående resa; returer läser ingen ny bön och ärver ingen bonus.
- **12 fysiska mutationer**: Thread-flagga/radie, Runner-fart, Stillness-fart/stormskydd/fraktskeppsstämpel/ägare, stående retur/returförbrukning, sena böner för verklig karavan/skepp, armébehörighet. Samtliga named red → byteåterställt → färsk gemensam grön före recepttillägget; nya fullsviten provar samma livscykler med större offer. [Resultat](mutations-b/results.json), [återställt](mutations-b/restored-fresh.log).
- **Fem nya fysiska receptmutationer:** för varje bön blir både en oberoende katalogassertion och verklig HTTP Offer-debit namngivet röda. Debit och lagrat RiteCast-offer provas för både lyckat och tyst svar, alla andra varor orörda. Byteåterställt/färsk Alpha+Beta-grön. [Resultat](mutations-recipes/results.json), [återställt](mutations-recipes/restored-fresh.log).
- **Full färsk Go efter recepttillägget grön:** [Go](recipes-full-go.log). **608 JS och vet gröna:** [JS](recipes-js.log), [vet](recipes-vet.log). Slutstatus i [receptverifiering](recipe-verification.json). Tidigare B-grundens fullsvit finns i b-full-go.log/checkpoint-b-verification.json. Den nya Go-loggens `+ändringar` gäller provverktyget, server/web är exakt242329a8.
- **Native Firefox1280/390 + Keryx:** healthz **242329a/mig176**, tom produktionsdiff. Fem beslutade recept, rätt gudsnamn, två verkliga Offer200/lyckade svar, båda pending och wait12 utan falsk varaktighet; inga JS-fel/HTTP-overrides. [Fakta](live-b-recipes/verification.json), [Keryx](live-b-recipes/keryx-after.txt).
- **BILD till Claude/Timothy:** [desktop](live-b-recipes/desktop.png), [pending](live-b-recipes/desktop-pending.png), [desktop avfärdsböner](live-b-recipes/desktop-departure-prayers.png), [mobil](live-b-recipes/mobile.png), [mobil böner](live-b-recipes/mobile-prayers.png). Ingen egen bildapproval påstås.

Spelarprovet använder normal register/join/grundning följt av **explicit egen PRE-ACTION SQL-fixtur**: tempel3, kharis90/rate0, grain/timber/fish/silver1000 (så två större offer och möjliga missar ryms). Ingen SQL under handlingarna. Det bevisar tempelinteraktionen, **inte naturligt tempelbygge eller ekonomisk nåbarhet**. Resornas beteende provas i DB/HTTP/worker-integration; inget naturligt stormväders- eller balansexperiment påstås för B. Egna runtime/containrar städade. Ingen merge/push/deploy från Codex.

## Checkpoint A — tidigare överlämning (nu mergad/LIVE 2ddee968)

Följande sammanfattning beskriver A vid dess överlämning; B ovan är aktuellt resultat.

Bas **6f89290b**, gren `codex/religion-alfa`, worktree `/home/tk/wt/codex-religion-alfa`.
**CHECKPOINT A KLAR + STOPP FÖR CLAUDES GRANSKNING.** Tal/inventering c52314c1 godkända 18:47;
produktion **ee0762cc**, migration **175**. Asterion/Stillhet hör till **B och är inte påbörjade**.

- Tre riktiga aktiva minoiska böner: Vision disk2/unknown-only memory, Walls ×1,25/6ticks på ägarens försvarande garnison, Green befintlig matlista/+25%/25ticks med förnyelseguard. Två godkända specs reserverade för B, ej kastbara/annonserade i A.
- Lyckad-only GodID+Wanax+värld-väntan i samma TX som gåva/debit/standing/RiteCast. Potnia24 delad/Velchanos25. Missar betalar men får **ingen** väntetid.
- Silvergolv100 endast i offervärdering. Gamla prayer-ID/specs/outcome-payloads orörda; pensionerade minoiska IDs kan inte nykastas. Övriga kulturers gamla regler bevarade.
- **Full färsk Go/tomDB/mig175, vet och605JS gröna.** [Go](a-full-go.log), [vet](a-vet.log), [JS](a-full-js.log), [verifiering](checkpoint-a-verification.json).
- **Nio fysiska mutationer** namngivet rött → byteåterställt och gemensamt färskt grönt. [Mutationer](mutations-a/results.json). Concurrent två städer och event-skrivfel/rollback provas genom riktig HTTP.
- Verklig Firefox/Keryx mot **healthz ee0762c/mig175**: Offer200, aktiv Walls6 och delad Visionwait24, explicit default/effekttext, ingaJSfel eller HTTP-overrides. [Fakta](live-a/verification.json), [Keryx](live-a/keryx-after.txt).
- **BILD-bunt:** [desktop](live-a/desktop.png), [desktop aktiv](live-a/desktop-active.png), [mobil](live-a/mobile.png), [mobil alla tre böner](live-a/mobile-prayers.png). Kräver Claudes/Timothys bilddom enligt överlämningskedjan; ingen egen approval påstås.

**Då kvar:** Claude granskar/integrerar A och ger separat START för B (utfört19:19). Ingen merge, push eller deploy från Codex.
Hela `megaron_religion_sammanstallning_20261012.md` läst före arbetet; kanon §5b/§5c och ordern gäller.

Claude 18:37 godkände tre uppmätta avgränsningar: Mistress behöver en faktisk garnisonsförsvarsläsare
(battle_frenzy har ingen i dagens combat), Green Shoots behåller grain/livestock/oil/wine och ej fish,
Asterion ger landkaravan radie2→1 och fart; bud får bara fart eftersom de redan är heliga.
Vaultens sammanställning §2 rättad för skördeeffekten. Stormscanens heltick-sampling är **inte i scope**.

**Traditionella offer — godkända av Claude 18:47 (B reserverades då):**

| Bön / gud | Spannmål | Timmer | Fisk | Silver | Varför |
|---|---:|---:|---:|---:|---|
| Potnia's Vision / Potnia | 2 | — | — | 4 | En liten matgåva + silver; kräver ingen oljepress/vingård. |
| Mistress of the Walls / Potnia | 3 | 1 | — | — | Mat till försvararna och ett byggmaterial; skog ger timmer utan förädlingskedja. |
| Green Shoots / Velchanos | 2 | — | — | 2 | Liten insats ur det man redan har, för att hjälpa produktionen. |
| The Thread Shows a Path / Asterion | — | 1 | — | 2 | En gubbes timmerarbete + liten silvergåva för nästa handels-/budfärd. |
| Favourable Passage / Havsdaimonerna i Stillhet | — | — | 2 | 3 | Hamnens fisk + silver; sjöbönen behöver ingen olja/vin. |

Varorna är **poolen av tidiga offer**, inte fyra obligatoriska ingredienser i varje bön:
alla fyra i varje recept skulle göra fisken till en spärr för inlandets första bön.
Spelarspråket är engelska; "Gynnsam passage" föreslås heta **Favourable Passage** i de fyra ytorna.

**Utförartal — godkända 18:47 utom den uttryckligen avslagna raden nedan:**

| Parameter | Förslag | Avgränsning |
|---|---|---|
| Silvergolv vid altaret | **100 divine value per silver**, före gudens smakvikt | `max(dagens silvervärde,100)` bara i offervärderingen; inte marknadspris, ny silverproduktion eller gods.base_value. |
| Mistress styrka/span | **×1,25 försvarsstyrka i 6 ticks** | Bara ägarens garnison som försvarar den välsignade staden; ingen anfallande armé eller fältstrid. Kopplas till befintlig styrka/tärningsberäkning, ingen ny stridsregel. |
| Asterion färdtid | **×0,75** | På nästa verkliga avfärdande landkaravan eller bud från staden; bevara karavanens befintliga ×1,5 före denna faktor. Inga arméer. |
| Asterion landrisk | **överfallsradie 1** | Godkänt av Claude; den nuvarande deterministiska radien är 2. Samma övriga sentry/FOW/regler. Ingen chans-tärning. |
| Stillhet färdtid | **×0,75** | Nästa skepp som lämnar hamnen, både riktig skeppsmarsch och skeppsbunden frakt. Bud ombord följer redan skeppet. |
| Stillhet skada | **ingen stormskada denna färd** | Fryst flagga; skaderutinen läser flaggan. Ingen ändring av sampling, stormtäthet, drift eller annan sjöfartsrisk. |
| Potnia väntetid | **24 ticks**, gemensam för Vision och Walls | En gud/Wanax/värld; två böner är alternativ, flera städer ger ingen extra kastplats. |
| Velchanos väntetid | **25 ticks** | Matchar den befintliga 25-tickseffekten och gör den inte möjlig att förnya på tick12. |
| Asterion / Stillhet väntetid | **12 / 12 ticks** | Avfärdsgåvor ska vara val, inte varje karavan. Ingen automatisk giltighetsgräns föreslås för oanvänd flagga. |
| Kharis-gate | **5 Vision/Walls/Thread/Passage; 30 Green Shoots** | Behåll befintliga två gates; ingen ny kharisskala eller ändring av 4-kharis-insatsen. |
| Väntetidens utlösare | **Lyckad-only, bevarad** | Claude AVSLOG väntetid efter tyst svar. Misslyckat kast tar offer/standing, ingen väntetid. Valideringsfel tar varken offer eller väntetid. |

Green Shoots har kvar **+25% i 25 ticks** för **grain, livestock, oil, wine**, aldrig fish.
Oanvänd avfärdsflagga och aktiv skörde-/försvarseffekt får inte skrivas över/förnyas av en ny bön.
För avfärdseffekter föreslås återbön i samma stad dessutom nekas medan den välsignade färden fortfarande
är aktiv; samma guds realm-väntetid gäller oberoende av stad. Detta är checkpoint B:s avfärdsscope, godkänt vid talgranskningen.
Utresans avfärdsleg är mottagaren; en senare separat returleg är en ny färd och får ingen ärvd ny fartgåva.
A:s tal är implementerade i ee0762cc; B:s katalogspecs är reserverade och inga avfärdskonsumenter/flaggscheman är byggda.

## Varför den storleksordningen

Aktuell dev-läsning under **BEGIN READ ONLY** (migration174, aktiv värld f7c90afa, tick150):
[råa värden](altar-values-live.txt), [processens healthz](dev-provenance.json): commit6f89290/migration174.

| Vara | Dagens divine value | Obyggd basproduktion per gubbe/tick (standardhex) |
|---|---:|---:|
| grain | 220,32 | 1 på plains, 1,667 i dal, 2,667 i delta |
| timber | 1468,8 | 1 i olivlund |
| fish | 293,76 | 1 i coastal_sea, 0,5 i flod/vad |
| silver | 1,7 | Kräver gruva för egen produktion; startgåva finns. |
| oil / wine | 146,88 / 122,4 | Förädlingskedjor, undantas från dessa fem traditionella recept. |

Kvantiteterna spannmål2–3/timmer1/fisk2 är arbeten på ungefär 1–3 standard-gubbeticks, före byggnadsbonusar.
Silver2–4 är en liten del av Hostens kodsatta 480-startsilver (minskar under vandring och kompletteras av
metropolisens genesis). Kolonin får grain8 och inget timmer/fisk i seeds: Green Shoots är avsiktligt liten,
men det krävs fortfarande tempel och kharis30. Inte ett påstående att varje ung stads första tick klarar alla offer.
Mat som offras konkurrerar alltid med mat som äts. Fartgåvornas värde växer med restiden, så ett långt
sjöben och stormfrihet är betydligt större gåvor än de små recepten; detta är **kalibreringsförslag**, inte uppmätt balans.

Med dagens Potnia-orakelvikter (grain0,6, silver1,4) skulle Vision-receptet vara värt
2×220,32×0,6 + 4×100×1,4 = **824,384**. Sex silver ger840 och är därmed ett meningsfullt
alternativ i CLI-kompositionen. Dagens gamla 20oil+15wine-orakel är värt **6242,4**;
utan golvet behövs cirka **2623 silver** för samma värde (1,7×1,4 per silver).
Golvet100 gör silver ett verkligt tidigt alternativ; offrets oddsmodifierare är fortfarande begränsad
+10%/−15%, så ett rikt offer ersätter inte kharis. Högre naturligt silvervärde klipps aldrig ned.
Gudernas nya smakkartor ska granskas/namnges tillsammans med katalogen; ovanstående räkneexempel
använder uttryckligen dagens vikter, inte en ny hemligt låst viktkalibrering.

## Slice-kontrakt

| Fält | Kontrakt |
|---|---|
| Problem | Minoiska böner har fel gudar, dyra förädlade offer, per-tempel-cooldown och saknar de beslutade avfärdsgåvorna. |
| Spelarsanning | Fem böner/fyra gudar med betalda och begripliga effekter, delad Potnia-väntan och fryst färdgåva. |
| Invariant | Historiska events tolkas inte om; nytt orakel ger bara tidigare okänd terräng som minne, aldrig live/rörlig aktivitet; en gåva påverkar aldrig pågående resa. |
| Scope | Religionskatalog/offervärdering, kast-/cooldownprojektion, FOW-minnesskrivning, transport/bud/ship-avfärd, försvarskonsument, fyra ytor och Codex. |
| Non-scope | Patrongud/nya tempelslots, Vrede/stormstyrning, ny intercepttärning, arméfartgåva, stormscanens sampling. |
| Acceptans | Alla fem effekter bevisade via riktig konsument + mutation; atomisk per-gud-väntan/ingen förnyelse; minnes-FOW; fryst avfärd/risk utan dubbelkonsumtion; web+Keryx och färska fulla sviter. |
| Stopvillkor | **Nu: A-hash + STOPP före B.** Kanonkonflikt till Timothy via Claude. |
| Bevisplan | Färsk DB-baslinje och riktade riktiga handler/workerprov; fysisk mutation för varje effekt, owner/FOW, delad cooldown och avfärd; fullfreshGo/JS/vet; Firefox1280/390 av templet, BILD i bunten. |

**Kedjegrind: bevisar** geografi → brist → brons → elit genom oraklets malmminne och genomförbara handelsfärder.
Första konkreta spelarytan: Kult → Prayers och `keryx rite --list`, därefter kartans grå minnespatch eller nästa färds ETA.

## Lagrade prayer-ID:n och historik

Komplett träfflista: [prayer-id-readers.txt](prayer-id-readers.txt).

- **Katalog:** `religion/prayers.go` har18 Go-specifikationer (6 kulturer×3); det finns ingen seedad prayer-tabell.
  Minoan-id:n är `minoan_oracle_deposits`, `minoan_harvest_blessing`, `minoan_battle_frenzy`.
  `CulturePrayers` och `AllowedForCulture` styr nya kast, medan `DefaultBattleFrenzyFor` konkatenerar ett ID.
- **Lagring:** `handlers/settlement.go:Rite` skriver `events.event_type='RiteCast'` under religionstreamen,
  `stream_id=settlementID`, med `player_id`, `prayer`, `effect_type`, `success`, offering, effect-utslag,
  värdering och `kharis_spent`. Inget GodID, cast_tick eller cooldown_until_tick finns i payloaden.
  Effekten lever som `battle_frenzy_until`/`harvest_blessing_until_tick`; inget prayer-ID på dessa kolumner.
- **Cooldown-läsare:** Rite `settlement.go:1063` och katalogens status `province.go:548` söker **senaste lyckade**
  RiteCast med player+prayer+settlement och mäter `created_at` via dagens kadens. Detta är **per tempel**, inte per Wanax,
  och created_at är loggtid, inte exakt cast_tick. Inget separat prayer-cooldown-schema finns.
- **Transaktionsgap:** kastet debiterar varor/standing och committar, sedan appendas RiteCast på poolen efter commit.
  En concurrent andra-stadskast kan passera en eventsbaserad väntetid innan det första eventet syns.
  Samma gömda källglapp gäller kast/status. Ny realm/gud-väntan måste låsas och skrivas i kast-TX.
- **Schemalagda prayers:** ingen sådan worker i dagens kod; böner utförs direkt. Orakel/harvest utfallet är inbäddat i RiteCast.
  `order_payload`/transportpayloads har inga prayer-ID-läsare; nya avfärdsgåvor ska bära frysta utfall/faktorer, inte en bön att slå igen.
- **Mätt dev:** läsning på alla världars RiteCast gav **0 rader**, och scheduled_events-payloads med minoan-ID gav0;
  [råläsning](stored-prayers-live.txt). Detta beskriver dev nu, inte andra databaser/äldre dumpfiler.

**Förslag till nya aktiva IDs** (nya semantiker/gud/offer, så gamla ID:n får inte tyst byta mening):

| Nytt ID | GodID | EffectType | Gammalt ID |
|---|---|---|---|
| `minoan_potnia_vision_v2` | `potnia` | `oracle_memory_v2` | `minoan_oracle_deposits` |
| `minoan_potnia_walls` | `potnia` | `garrison_defense` | `minoan_battle_frenzy` pensioneras som Poseidons gamla gåva. |
| `minoan_velchanos_green_shoots` | `velchanos` | Befintlig `harvest_blessing` | `minoan_harvest_blessing` pensioneras som Britomartis. |
| `minoan_asterion_thread` | `asterion` | `next_land_departure` | Ny. |
| `minoan_stillness_passage` | `sea_daimons_stillness` | `next_ship_departure` | Ny. |

Gamla RiteCast-payloads/effektnamn lämnas byteorörda. För gamla minoiska IDs föreslås bevarade legacy-specs
för historisk läsning men inte i aktiva `CulturePrayers`; nya kast med ett pensionerat ID får ett tydligt svar
med nya namn. Övriga kulturers18-minus3 gamla böner är orörda. Minoans tomma-body-default blir Walls;
CLI:s `strings.HasSuffix("_battle_frenzy")` i bekräftelsen måste ersättas av serverns explicita default-ID.
`capabilities/cult_verbs.go` måste använda samma default. Ingen ny verb/Umami-event behövs.

**Per-gud-väntan:** föreslagen liten, tickbaserad projektion nycklad på world+player+GodID (alternativt GodID→until_tick
på PWR). Samma PWR-lås som standing säkrar samtidighet; projektion + gåva + debit + outcomes-event i samma TX.
Status läser samma projektion. Gamla historiaevents ska inte redigeras; vid andra DB:er med existerande cast-historia
krävs uttrycklig legacy-mapping och övergångsregel eftersom exakt tick inte går att återskapa efter kadensbyte.
Dev har inga casts att backfilla, men det är inte skäl att anta att alla installationer saknar dem.

## FOW-minnets lagring och rätt skrivväg

[Träfflista](memory-readers.txt). `player_scouted_tiles` (mig068, world-FK mig104) har world_id/player_id/q/r,
scouted_at och uniknyckel(world,player,q,r); **inga liveögon, inga enheter och inga aktiva bygg-/tränings-/stridsfält**.

`handlers/world.go:1046 loadRememberedTiles` läser unionen av:
1. `player_scouted_tiles`,
2. `player_scouted_provinces` via provinskoordinater,
3. egna levererade/returnerande/hemkomna budkontakter via destinationsstaden.

`/map` låter **egna levande ögon vinna**, därefter unionens minne → `tier="remembered"`, annars fog.
API representerar alltså tier2 med strängen remembered; klientens renderer grålägger minnet.
Terräng/deposits är koordinatminne läst mot statiska map_tiles, inte per-spelare snapshots av rörlig värld.
Detta kräver inga nya minnestabeller eller ny live-källa.

Dagens orakel `settlement.go:1369` lottar en ore-hex som **inte finns i player_scouted_tiles** och skriver
hex+radie1 (7 möjliga hexar) till samma tabell. Det kontrollerar varken faktisk LIVE-sikt eller unionens andra
minneskällor, och readback-payloaden listar hela patchen även om vissa hexar redan var kända.
Det får inte bara bli en radius1→2-ändring.

**Ny plan:** filtrera kandidater mot hela unionens minne **och AnyEyeSees med samma terräng/sjöhorisont**;
lottning fritt över resterande ore-hexar (inte preferens för saknad metall). Skriv från riktig map_tiles-**disk2**,
max19 hexar, endast tidigare okända/icke-live rader. `ON CONFLICT DO NOTHING`; utfallet listar det som verkligen
blev nytt. Kartkant ger färre; hav i patchen är normal markdata. Skriv inga liveögon, sighting av trupper/stormar,
ryktesnyheter eller aktivitet. Finns ingen ny fyndighet ska svaret säga det ärligt.

Minnesaktivitet behöver testas genom /map, /provinces, foreign-units och storms:
`world.go:979` gömmer redan ArmyTotal/Burning och all foreign build/train vid minne, men **Besieged** kommer
från aktuell DB-rad och är inte nollad i den minnesgrenen. Ett nytt omen-minne kan därför annars läcka pågående
belägring. Rekommendation: samma icke-live-guard döljer den aktiviteten, och ett prov namnger detta (inget live-läckage).
Ingen omskrivning av statiskt kartminne till en ny snapshotsmodell föreslås.

## Effektkonsumenter och avfärdslivscykel

[Träfflista](effect-consumers.txt), [samtliga avfärdsskrivmönster](departure-writers.txt).

- **Mistress:** gamla `battle_frenzy_until` skrivs i Rite och läses av API/UI/godview, men **ingen production combat-läsare**.
  Faktiskt battle_tick läser participants, loyalty/kharis/fortify och rullar befintlig styrka/tärningar.
  Ny defense-flagga i ticks måste läsas enbart för den välsignade stadens ägares defenders vid dess koordinater,
  aldrig attacker, annan stad, allierad armé på fält eller redan bortmarscherad garnison. Gamla flaggan aktiveras inte
  retroaktivt. Claude har godkänt verklig consumer; multiplicator1,25/6ticks väntar talgranskning.
- **Green Shoots:** `economy.ApplyHarvestBlessing` använder GREATEST och kan idag **förnya** en aktiv effekt.
  Ny bönguard måste stoppa förnyelse före debit/rullning. Behåll yieldmotorn och separat gudomlig slumpvälsignelse;
  ingen bred ändring av kharis-tickens gavor utan behov.
- **Asterion karavan:** allt goods-dispatch går genom `transport.Dispatch/CreateShadow → prepareJourney`;
  förplanerade journeys kommer också från handlers (transfer/gift/trade accept/kingdom) och standing_orders/retur/recovery.
  Ruttens duration behöver fryst faktor med default1 för gamla JSON, annars skulle dagens `TradeJourney.Validate`
  förkasta rabatterad duration. Sparade kostnader/progress, planner-ETA, scheduled events och payloadens ETA måste
  få **samma** slutliga duration. Valideringen får inte bara släppas lös. Atomisk pending-claim på verklig avfärd;
  samma TX rollback återställer gåvan om debit/job misslyckas. Inget dubbelt konsumtionsställe mellan planner och Dispatch.
- **Asterion bud:** `messenger.PlanOutboundRoute`, `PlanReturnRoute`, InitOutboundPassage/retur och
  completion-generation är resans gemensamma livscykel. En planering är **inte** en avfärd. Spara tidsfaktorn på budet;
  använd den i verkligt landben/korrekt completion-job och i mötespunktsräkningen där order jagar en rörlig enhet.
  Väntan i hamnen har ingen egen skeppsfart. Ombord följer budet bäraren; ingen dubbel0,75 om två gudar välsignat olika ben.
- **Stillhet skepp:** både `combat.StartMarch/TravelFactor` och skeppsbunden varufrakt måste förbruka samma stads gåva
  precis en gång. Freighting-skeppet rör sig genom transportens sparade journey, inte en samtidigt egen march_route.
  Spara fartfaktor och no_storm_damage på den **aktiva färden**, så nästa oblessed färd inte ärver den via stale ship-row.
  Return/recall/redirect/damaged-return är nya leg-dispatchers; de omprissätter aldrig pågående ben.
  Stormskadan läser endast den sparade flaggan; heltick-sampling/RouteEnterMilli/drift lämnas som de är.
- **Ägarskifte/kollaps:** en pending stads gåva får inte övergå till erövraren. Flaggan måste bära den ursprungliga
  Wanaxen eller rensas i ägarskiftets shared owner-seam. Resan bär sitt frysta utfall, inte en ny lookup av stadens ägare.
  Mina schemaförslag är ännu inte låsta; inventera alla owner/tillståndsskrivare under implementationen.

**G1:** combat och transport får idag inte importera religion enligt architecture_test.g1Allowed.
Ingen ny förbjuden import föreslås. religion äger katalog/offer/GodID; transportation äger den generiska
avfärdsprojektionen och claimen som kan läsas nedåt av combat/messenger/economy. province äger sparat
journeyformat/duration-validator och får inte importera transport/combat. API orchestrerar bönens TX.
Tuningens källa ska vara en per parameter, de frysta resorna lagrar det beslutade talet; inte dubbla nya kataloger.

## De fyra ytorna och kända baslinjefynd

- **Temenos:** Rite + available_prayers behöver samma per-gud-/aktiv-/affordability-regler; defaulten måste bli explicit.
  Verklig consumer ska ge outcome-event, inte "will apply later". Riktade scenarier har redan route/planner/harvestprov att bygga på.
- **Keryx:** `cmd_rite.go` läser katalog/cooldown_game_days från servern; default-bekräftelsen gissar idag suffixet
  `_battle_frenzy`. Ska läsa explicit default och kunna visa alla fem nya effekt-/flaggsanningar.
- **Webb:** `ui/drawers/kult.js` läser dynamisk katalog; ingen handskriven minoisk prayerlista.
  Den läser dock **gamla cooldown_remaining_minutes**, medan servern skickar **cooldown_remaining_game_days**.
  Resultat: afford=false kan visas som "not affordable" i stället för rätt väntetid. Rättas i samma väntetidsyta, inte en separat buff-UI.
  Varje bön visar effektbeskrivning och pending/active-status så spelaren kan välja nästa färd. Firefox1280/390 till BILD-bunten.
- **Codex:** rites, runners/messengers, routes/trade/transfers, sea/storms och battle måste beskriva de nya faktiska reglerna.
  Nya effektIDs är inte nya verb. Verblistans rite-rad uppdateras; Umami "rite" bevaras, inga nya verb-event.
- **Legacy:** 18-böner/3-per-kultur-invariant i religionstesterna måste begränsas till de fem oförändrade kulturerna;
  minoan-listan ska vara5. Gamla oracle-seven-patchproven ska fortsätta testa legacy-effekten separat från nya memory_v2.

## Baslinje / grindar

Färsk tom DB på migration174, oförändrad **6f89290b**:
`tools/gotest.sh ./internal/religion/ ./api/handlers/ -run 'TestPrayer|TestDefaultBattle|TestCultureGate|TestOffering|TestOracle_|TestRite|TestHarvestBlessing'`
— **gröna**, [baseline-fresh.log](baseline-fresh.log). Inga produktionspatchar i denna checkpoint.
Dev-läsningen ovan var separat READ ONLY; det är ingen spelacceptans eller balanssoak.

Kod: katalog/minnes-/harvest-baslinje grön. Semantik: lagradeIDs, verkliga konsumentglapp och väntetidskällan inventerade.
BILD/användarflöde/fullsviter: återstår efter godkända tal och implementation. Drift: ingen merge/push/deploy.

**Nästa handling:** Claude granskar A/BILD, integrerar och ger separat START för B. Asterion/Stillhet och stormsampling är orörda.

## Checkpoint A — genomförande och slutbevis

Migration175 skapar GodID-väntetid med nyckel(world,player,god) och world/player-cascade;
försvarsgåvan har until_tick och owner-stämpel på staden. Inga historiska events/legacy-flaggor skrivs om.
Ny-prayer-ID-versionen börjar med ny väntetidsprojektion; den använder inte äldre cast-loggtid som falskt exakta ticks.
De pensionerade minoiska IDs/specs finns kvar för historia, men bara de tre implementerade nya är i CulturePrayers.
De övriga fem kulturerna behåller sina befintliga per-tempel-event-väntetider. B:s två godkända spec-ID:n är ännu inte aktiva.

Rite tar samma Wanax/PWR-lås över båda städerna, och outcome-event appendas med AppendTx.
Lyckad bön skriver väntetid; miss gör det aldrig. ActiveRemaining och GodCooldown har gemensamma läsare i cast/status.
CLI väljer serverns is_default och visar aktiva dagar + wait; Kult visar serverns game_days, effekttext och rätt Offer-guard.
Ingen ny verb/Umami-kind. Rites/Battle i spelar-Codex, rite-pariteten och religions-vaulten uppdaterade i A.

Vision har egen oracle_memory_v2-consumer, medan den gamla sjuhaxseffekten förblir kvar.
Kandidater filtreras mot faktisk live-sikt och HELA minnesunionen (tiles/provinces/budkontakter).
Disk2 skriver bara nya riktiga hexar (INSERT/ON CONFLICT), och payloaden listar bara faktiskt nya minnesrader.
Real /map-provet visar19remembered, /provinces-provet visar minnesstad utan ArmyTotal/Besieged.
Ingen storm-/unit-sighting eller live-eye skapas. Noll ny fyndighet får ärligt svar.

Walls läses i den riktiga BattleTick/rollSide-vägen för eligible garrison/defender/stad/owner/expiry.
Ett real siege-prov jämför likadan seed med/utan gåva; egna defenders får exakt ×1,25 dice,
attacker får samma dice, fel owner/utgången gåva/borttagen settlement-länk får noll bonus.
Gammal battle_frenzy_until får ingen retroaktiv combatbetydelse. Green går genom befintlig
ApplyHarvestBlessing/RecomputeProduction och blockeras före debit när active; befintliga hela yield-/fishprov passerar.

Mutationer: Vision-radius; Walls-consumer; Walls-owner; Green-span; Green-förnyelse;
miss-väntetid; delad-Potnia-key; silvergolv; Besieged-i-minne. Varje har named-red-logg och byte-restoration;
alla Alpha HTTP/workerprov omkörda mot ny tom DB i restored-fresh.log. Fullsuite på ee0762cc/mig175 efter slutkod är grön.

Firefox-beviset skapade egen färsk PG16/Redis och körde nybyggd server/Keryx, isolerad miljö, tick60s.
Normal register+join+grundning **Phaistos**, sedan EXPLICIT SQL-fixtur endast på den egna nya staden:
tempel3, kharis90/rate0, grain/timber/fish/silver100. Detta är INTE ett naturligt bygg-/kult-ekonomibevis.
Efter fixturen sker ingen SQL eller HTTP-override: native Offer-clicks använder riktiga requests/utfall,
missar skulle få försöka igen utan wait; success ger Wallsactive6/Visionwait24 och riktig inline-svarrad.
Keryx --list läser samma process före/efter. Alla4 bilder är nya från sista rena scenariot med brief stängd;
mobile-prayers är skrollad i själva Kult-drawern. Alla processer/container resurser var egna och stängdes.

Provverktyget rättades under arbetet för hexmodellens avsaknad av unik(settlement,building_type)
och för CLI-konfigurationens province_id. De första riggförsöken hann inte prova något Offer och används
inte som bevis; a-live-clean.log och live-a/ är slutbeviset. En gotest-container fick portkrock före DB/teststart;
färsk omkörning grön (a-effects-retry.log), ingen testregression normaliserad.

Repro: tools/gotest.sh; go -C server vet ./...; node --test $(rg --files web/static/js -g '*.test.mjs');
python3 tools/religion_alpha_mutations.py; python3 tools/religion_alpha_live.py.

Grindar A: kod/semantik gröna, faktisk tempelinteraktion/API+Keryx+Firefox grön med angiven fixtur.
BILD levererad för separat dom; drift väntar Claude. Checkpoint B är inte påbörjad.

## Checkpoint B contract (START 2026-10-12, base 2ddee968)

Problem: the two reserved travel prayers have no departure consumers. Player truth: a successful offering waits in its city for one eligible outward departure. Invariant: owner-stamped pending blessings are consumed transactionally once; saved journeys, armies, return legs and storm sampling retain their behavior. Scope: one pending-departure projection, frozen journey metadata, caravan/Runner/ship departure consumers, existing interception/storm damage readers and four surfaces. Acceptance: five usable prayers; factor 0.75 at departure; land caravan radius1; Stillness no damage only that leg; late cast and return inheritance tests with physical mutations. Fresh baseline then full fresh Go/JS/vet plus native Firefox/Keryx proof. Hash and STOPP; Claude integrates. Supports the trade part of the chain, does not prove a natural full economic progression.

## B — lifecycle, grindar och avvikelser

Alla transport.Dispatch-writers går genom samma consumption. Shadow-writers för intern transfer/gift och bilateral trade förbereder gåvan **före** ETA, fryst journey och schemaläggning; stående order gör samma för utresan. Returjourneys planeras separat och får inget av utresans metadata. Gamla JSON utan nya fält använder samma ursprungliga faktor1 och kalibrering; historiska payloads och StepCosts skrivs inte om.

Runner Send (inklusive offerbud) och sendOrderCourier fryser den första avfärdslegens ETA i samma insert/schedule-TX som förbrukningen. En väntande budpassage behöver verkligt skepp; dess sjöleg/retur och senare landleg konsumerar inte Asterion. Skeppets StartMarch låser status och konsumtion före tidskontrollerna; preview läser pending men konsumerar aldrig. Expeditionens returnresolver använder samma TX för sina FOR SHARE-lås. Return/redirect/limp rensar skeppsstämpeln, medan stormvakten också kräver exakt voyage och nuvarande ägare.

Färsk baslinje på2ddee968/mig175: [baseline](b-baseline.log). Riktade finalprov Beta/Alpha/Expedition: [final](b-targeted-restored.log). Testerna omfattar faktisk transport.Dispatch/schemaläggning, freighting-skepp, riktig Messenger Send-HTTP, verklig StartMarch+preview+dispatchLeg-retur, riktig stående retur, prayer-after-actual-departure, owner/rollback och fortsatt pending efter return. Stormproven använder avsiktligt lagrade testspår från den befintliga testfixturen, inte naturligt weather/seedad riskandel.

Granskningen hittade två implementationfel före slutkod: stående return preflight hade fel outward=true; expedition preview och execution använde olika bonus före tidsvakterna. Båda rättade; riktade prov gröna. Flytten till avfärds-TX exponerade först en self-blocking FOR SHARE via pool i expeditionreturn-resolvern; den använder nu samma TX, fresh Expedition-gruppen och fullsviten provar det. Egna blockerade testprocessen avbröts/städades, ingen delad process. Initiala testfixturer korrigerades för aktuella RÖ1-kostnader/signaturer och riktig sammanhängande karta, ingen balansändring.

En fysiskmutationskörning avbröts av upptagen testport före DB/teststart; därefter väljs ledig port per mutation och alla12 namedred samt restored-green kördes. Arkiv b-mutations-initial-port-collision.log; b-mutations.log är slutkörningen. Native UI-riggen upptäckte dolt Godnamn i Keryx när Favours saknas (rättad display) och omitempty active0 (rättad assertion). Båda före slutprovet. Precommit-resultat är arkiv; live-b och b-live.log är omkörda mot3858cfab.

Repro: tools/gotest.sh; go -C server vet ./...; node --test $(rg --files web/static/js -g '*.test.mjs'); python3 tools/religion_beta_mutations.py; python3 tools/religion_beta_live.py. Grindar: funktion/semantik och faktisk UIinteraktion med angiven fixtur; BILD ligger i bunten, integration/drift ägs av Claude. **Hash och STOPP.**

## Recepttillägg — utförande och repro

Tillägget19:33 kom i agentlogg under B men levererades till Codex-turen efter den första överlämningen20bf4cd4. Separat commit242329a8 ersätter endast de aktiva katalogrecepten; äldre events/offering-payloads är fortfarande frysta. Codex-artikeln visar samma nya siffror. Testfixturen för Alpha har silver500 i stället för100 för de upprepade avfärdskasten, medan övriga varor förblir100; Walls silent-debit-assertion är nu92 efter ett offer8. Ingen produktions-stock/startregel ändras.

`TestAlphaTraditionalRecipesOctoberDecision` har fem bokstavliga beslutade maps som är oberoende av katalogen. `TestAlphaOfferDebitsDecisionRecipeExactly` kör riktiga default-Offer-HTTP-kast för alla fem, både answered/silent, läser alla lager före/efter och samma TX:s RiteCast-offer. Mutationer sänker en ingrediens för varje bön: Visiongrain10→9, Wallstimber4→3, Greengrain15→14, Threadsilver100→99, Passagefish6→5. Varje måste ge namedred i BÅDA testgrupperna och återställas byteexakt före färsk gemensam grön.

Ny native-rigg med health242329a/mig176 och tom produktionsdiff läser katalogens samtliga maps, jämför dem i CLI-listan och den verkliga drawerns text. Offer-knappen skickar fortfarande endast prayer-ID och servern räknar det traditionella offret. Två verkliga Thread/Passage-Offer200 lyckades; pending+wait12 visas. Fem nya slutbilder ligger i live-b-recipes; äldre live-b är föregående receptversions bevis och används inte som bevis för de större recepten. Egen fixturestock1000 inför handlingarna är explicit, inte ett naturligt ekonomibevis.

Repro för tillägget: python3 tools/religion_recipe_mutations.py; tools/gotest.sh; go -C server vet ./...; node --test $(rg --files web/static/js -g '*.test.mjs'); RELIGION_PROOF_OUT=live-b-recipes python3 tools/religion_beta_live.py. Migration fortsatt176. Alla uppdaterade slutprov gröna; hash och STOPP.
