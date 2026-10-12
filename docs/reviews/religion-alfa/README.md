# Religion-alfa — inventering och talförslag före kod

## Sammanfattning / granskningspunkt

Bas **6f89290b**, gren `codex/religion-alfa`, worktree `/home/tk/wt/codex-religion-alfa`.
**INVENTERING + FÖRSLAG + STOPP. Ingen produktionskod eller offertal låsta.**
Hela `megaron_religion_sammanstallning_20261012.md` läst före arbetet; kanon §5b/§5c och ordern gäller.

Claude 18:37 godkände tre uppmätta avgränsningar: Mistress behöver en faktisk garnisonsförsvarsläsare
(battle_frenzy har ingen i dagens combat), Green Shoots behåller grain/livestock/oil/wine och ej fish,
Asterion ger landkaravan radie2→1 och fart; bud får bara fart eftersom de redan är heliga.
Vaultens sammanställning §2 rättad för skördeeffekten. Stormscanens heltick-sampling är **inte i scope**.

**Föreslagna traditionella offer (inte beslutade):**

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

**Andra tal som behöver Claudes granskning innan implementation:**

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
| Väntetidens utlösare | **rekommendation: varje giltigt kast, också tyst svar** | Nytt mot dagens lyckad-only. Ska uttryckligen godkännas; alternativet är behåll lyckad-only. Ingen vänta/debit på valideringsfel. |

Green Shoots har kvar **+25% i 25 ticks** för **grain, livestock, oil, wine**, aldrig fish.
Oanvänd avfärdsflagga och aktiv skörde-/försvarseffekt får inte skrivas över/förnyas av en ny bön.
För avfärdseffekter föreslås återbön i samma stad dessutom nekas medan den välsignade färden fortfarande
är aktiv; samma guds realm-väntetid gäller oberoende av stad. Detta behöver bekräftas tillsammans med talen.
Utresans avfärdsleg är mottagaren; en senare separat returleg är en ny färd och får ingen ärvd ny fartgåva.
Inget tal ovan är inlagt i katalog, DB eller produktion.

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
| Stopvillkor | **Nu: granska offertal/effekttal, väntetidens kast-utlösare och avfärdseffektens leg-scope före kod.** Kanonkonflikt till Timothy via Claude. |
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

**Nästa handling:** Claude granskar offer/silvergolv/styrka/fart/väntetid/leg-scope och ny FOW-aktivitetguard,
därefter implementation på samma gren. Hash + STOPP gäller tills besked.
