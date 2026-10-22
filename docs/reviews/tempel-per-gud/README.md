# ORDER 15 — inventering och förslag, ingen implementation

## Sammanfattning och kontrakt

Inventerat 2026-10-22 mot ORDER13 `codex/tempel-gubbar` **b2729e15**, egen dokumentgren `codex/tempel-per-gud`. Detta är statisk källanalys, inte en mätning av körande server. Ingen live-DB läst eller ändrad, ingen migration/rebase av ORDER13, ingen produktionskod eller klienttext ändrad. ORDER14/16 är avslutade enligt Claude; deras äldre kömeddelanden ignoreras.

Problem: en stads enda generiska tempel ger tillgång till kulturens böner. Spelarsanning enligt ORDER15: flera tempel, ett per gud och stad; rätt gud och nivå låser upp böner; varje tempel heter efter sin gud. Inventeringen föreslår ett sammanhängande byggnads-ID genom bygge, bemanning, rivning, offer och bön. Kharis förblir en rikspool. **STOPP före kod och kanonbeslut.** Arbetet väntar på ORDER13:s merge/BILD och beslut; bevisar inte kedjegrinden geografi→brist→brons→elit.

Källor: `.agents/order-tempel-per-gud-20261022.md` inklusive namntillägget19:00; vault `temenos_pantheon_tempel_riter.md`, `temenos_kharis.md`, `megaron_arbetssatt.md`. Pantheondokumentet är märkt designunderlag: dess gamla regler får inte automatiskt ersätta senare fungerande spelregler.

## Baslinje: vad koden faktiskt gör

| Ämne | Fynd och källa på b2729e15 |
|---|---|
| Tempelobjekt | `buildings.id` finns redan. Migration154 rader83–86 gör stadsbyggnad unik på `(settlement_id,building_type)`; därför högst ett generiskt temple. Ingen gud på byggnaden eller köposten. |
| Gammal temples-tabell | Borttagen av migration135 som dött parallellt substrat. `religion/model.go` har en historisk Temple-modell med pantheon/localpower, inte en fungerande gudstillhörighet. Återuppliva inte den som andra nivåägare. |
| Gubbar efter13 | `economy/temple_staffing.go`: MAX(temple.level), count av alla building/temple/cult-placeringar, 2×nivå, min(cap,placed). `placement_yield.go:464` erbjuder en aggregerad temple-rad. Placeringar saknar byggnads-ID (migration120). Två tempel kan alltså inte bemannas separat med nuvarande kontrakt. |
| Byggslut | `combat/build.go:80–89` upsertar efter typ/hex, stadsvariant efter typ. Ett nytt tempel skulle annars uppgradera fel tempel. Kö/event/quote måste bära gud och avsedd instans. |
| Bön | `handlers/settlement.go:783–810`: kulturtillhörighet + EXISTS valfritt tempel. Ingen tempelnivå i PrayerSpec. `province.go:451–570` bygger tillgänglighet separat. `capabilities/cult_verbs.go` kontrollerar kulturens standardbön, kan annars lova en otillgänglig handling. |
| Cooldown | De fem aktiva minoiska bönerna har GodID; cooldown är per värld/Wanax/gud. Äldre kulturkatalog har tom GodID och legacy eventbaserad per-stads/per-bön-cooldown. Inget får retroaktivt ändra innebörden av gamla RiteCast. |
| Kharis | `kharis/tick.go:153–166` använder tempelstäder och rikets MAX-nivå; economy äger fysisk bemanning. `project.go` måste matcha tick. 1,05 per verklig tempelgubbe före offer/knapphet/börda enligt13; ingen ny godsvis kharispool. |
| Offer | `kharis/temple_offering.go:95,140`: EXISTS tempel, en plan/debit per stad. Flera tempel kräver beslut om kostnad per tempel. Fristående planer får inte alla lova samma sista korn i stadens lager. |
| Grundning | `handlers/create_metropolis.go:193` grundar utan tempel (gård eller galär som separat gåva). Koloni `combat/unit_arrival.go:1924ff` har kultur men inget patronval eller gratis tempel. Ingen beständig patrongud hittad. Underlagets gratis patrontempel är därför en verklig speländring. |

Råträffar finns i `surface-grep.txt`, `schema-grep.txt`, `lifecycle-grep.txt`. De är sökunderlag, inte påståenden att alla träffar ska ändras. Historiska migrationer/events och äldre böne-ID:n behålls som historik; andra byggnaders typbaserade produktionsregler behålls. Testträffar är framtida fixtures/regressioner. Renderträffar kräver kontroll av antalet instanser och etikett, inte nya sprites eller ny gudsmekanik automatiskt.

## Föreslagen datamodell och regelägare

**Rekommendation: behåll varje tempel som en riktig rad i buildings, lägg god_id där.** En temples-tabell med egen level skulle dubblera bygg-, rivnings- och nivåägarskapet. Om separat metadata senare behövs kan den vara 1:1 på building_id och sakna egen nivå.

- Gudskatalog i religion: stabilt ID, visningsnamn, tillåtna kulturer och koppling till befintliga böner. Inte strängnormalisering av `spec.God`, som är visningstext. Gudar/aspekter behöver uttryckliga ID:n.
- Datakrav: temple ⇒ giltigt god_id; andra typer ⇒ inget god_id. Behåll övriga stadsbyggnaders unikhet, skapa separat unikhet `(settlement_id,god_id)` för tempel. FK/validering för giltig gud och kultur måste ha en entydig ägare. Ingen klientgodtycklig gudssträng som displaynamn.
- `build_queue`, build-kommando, quote, event och build completion behöver gud för nybyggnad och stabilt byggnads-ID för uppgradering. Unikhet/settlementlås stoppar två samtidiga byggen av samma gud. Beräknad nästa nivå gäller det valda templet, inte stadens högsta tempel.
- Tempelplacering behöver `building_id`, samma stadsägare och rätt måltyp. Tillför detta smalt för tempel först; andra arbetsplatser kan behålla typkontraktet tills deras behov ändras. Typen `temple` och `cult` kvar som klassificering, inte identitet.
- Economy fortsätter äga cap och faktisk staffing **per tempelinstans**. Rikets summering summerar de giltiga instanserna. Flytt/rivning prunar bara gubbarna på rätt tempel, högsta ordinal först; flyttar inte människor tyst mellan gudar. Kapacitet från ett annat tempel får aldrig rädda en ogiltig placering.
- Religion äger en ren behörighetsregel (gud, nivå, böne-ID). Högre lager läser byggnad/tillstånd och använder samma regel i cast, status och capabilities. Economy behöver inte importera religion uppåt i paketgrafen. Kharis fortsätter använda economy för staffing och sin befintliga regel för rikspool/ceiling.
- API bör exponera `temples[]` med building_id, god_id, god_name, level, display_name, staffed/capacity och lokala offer. Böner anger nödvändigt tempel, nivå och blockeringsorsak. Befintliga sammanfattningar kan vara uttryckliga summor, aldrig välja första templet i tysthet.

## Övergång och livscykel

Befintliga tempel kan inte få en historiskt belagd patrongud: inget sådant val är lagrat. Att ta kulturens första gud skulle göra ordningen i en lista till kanon. **Ingen automatisk backfill innan Timothy valt princip.** Föreslå samma nivå/ID/bemanning bevarad när guden tilldelas; inga dubbla tempel eller extra gubbar som kompensation utan beslut.

För en beslutad kulturgudsmappning: redovisa torrmanifest för samtliga berörda världar, tempel, köposter och tempelplaceringar; okänd kultur/gud ska stoppa övergången, inte fallbacka. Koppla ORDER13:s befintliga temple/cult-placeringar till stadens enda befintliga tempel-ID. Backfilla pågående temple-byggen uttryckligen, bevara nivå/kostnad/ETA. Atomisk uppstart och idempotent omkörning; inga workers/HTTP på halvt migrerat tillstånd. Migrationens nummer väljs först vid integration: ORDER13:s199 måste bli201 enligt Claude, ORDER15 tar nästa då lediga nummer.

Kontrollera alla vägar i lifecycle-grep: metropolis, foundColony, uppgradering/köcancel/slutförande/retry, demolition, stormskada, capture/ägargåva, revolt, sunk/collapsed/razed och eventuell återaktivering. Tempel-ID/gud följer objektet vid ägarbyte enligt beslutad kulturpolicy; staffing hör fortsatt till stadens arbetsstyrka. FK får inte lämna hängande placeringar när en byggnad försvinner. Pågående gudscooldown tillhör Wanaxen, inte en säljbar byggnad. Historiska event tolkas inte om; nya utfall bär tempel/gud/nivå där det behövs. Alla generiska building_type='temple'-läsningar klassas som antingen existens, antal tempel, antal tempelstäder, MAX-nivå eller instans — dessa är olika frågor.

## Vilka befintliga böner skulle vilket tempel låsa upp?

Minoernas aktiva katalog i `religion/prayers.go:263–279` är fem böner/fyra gudar:

| Tempel | Befintliga böner | Nivå idag / förslag före beslut |
|---|---|---|
| Potnia | Potnia's Vision; Mistress of the Walls | Ingen nivågrind idag. Föreslå båda L1 i första slice för att inte smyga in en ny nerf. |
| Velchanos | Green Shoots | Ingen nivågrind; föreslå L1 tills Timothy beslutar annat. |
| Asterion | The Thread Shows a Path | Ingen nivågrind; föreslå L1 tills beslut. |
| The Sea Daimons of Stillness | Favourable Passage | Ingen nivågrind; föreslå L1 tills beslut. |

Designens L1-bön/L2-rit/L3-åkallan är inte en beslutad mappning av dessa fem. Namnet ”rit” i nuvarande UI är inte bevis för L2. Inga nya L3-effekter, murkrossning, risker, kharisdrag eller cooldownändringar ingår i förslaget. Aktuell kod säger MinKharis är krav, inte förbrukning; det äldre designunderlagets kharisdrag införs inte här.

Övriga kulturer har tre aktiva äldre böner vardera: akhaier Apollon/Demeter/Ares; khemetiu Thoth/Osiris/Sekhmet; knaani El/Baal/Anat; thrakes Sabazios/Bendis/Ares Thrakios; hatti Hepat/Telipinu/Teshub. De saknar GodID och nivåkrav. Föreslå explicit katalogkoppling utan nya effekter; besluta omfattningen först. Äldre minoan_oracle_deposits/harvest_blessing/battle_frenzy finns som historiska specs men är inte den aktiva minoiska listan — ge dem inte oavsiktligt nya knappar. Pantheontabellens namn/aspekter avviker delvis från spelkatalogen och måste mappas uttryckligen.

## Fyra ytor och namn (tillägget19:00)

**Rekommenderad fullform: `Temple of Poseidon L2`, `Temple of Potnia L1`.** Serverns strukturerade god_name/level äger innebörden. I befintlig byggnadstabell med separat nivåkolumn: namncell `Temple of Poseidon`, nivåcell `L2`, samma form som andra byggnader (`city.js:483`). I kompakt lista, arbetsplatskort, offertitel och notis: fullformen. Gruppnamnet ”Temples” får finnas men kan inte ersätta instansnamnet vid en handling.

| Yta | Vad som behöver bli instansspecifikt |
|---|---|
| Temenos | byggkatalog/quote/queue/completion, byggnadslistor, placements/options, demolish, status/temple_offers/available_prayers, capabilities/actions, notifications och fel. Alla hänvisar till samma god_id/building_id. |
| Keryx | build --list/nybyggnad/uppgradering, staff/list/unstaff, demolish, status/cities/rite --list/kvittens/notiser/help. Föreslå --god vid nybyggnad och --building för entydigt befintligt tempel; `staff temple +1` får kräva val vid flera tempel, aldrig välja ett godtyckligt. CLI-formen är förslag, inte implementerad. |
| Megaron | City byggval/lista/uppgradering/kö/rivning, citygrid arbetsplatskort, Kult temple offerings/bönelista/krav/fel, notiser. Renderer citybuildings/citysprites måste granskas för flera temple-instanser; ny grafik ingår inte automatiskt. Långt gudsnamn provas390px och desktop. |
| Codex | buildings, cult/rites/kharis samt befintliga staffing- och grundningsinstruktioner: ett per gud, val och bemanning av rätt tempel, nivåkrav och riksgemensam kharis. Dokumentera bara byggda riter, inte hela framtida pantheonet som knappar. |

ORDER17 ändrar serverägd byggquote och nivåvisning på dessa ytor; samordna dess slutkontrakt före ORDER15-kod. ORDER13 bemanningsmigrering och dess klient/API-kontrakt är substrat. ORDER14/16 behöver inte byggas om; deras integrerade master används vid framtida rebase.

## Kanonfrågor till Timothy — i spelarspråk

1. **Vilka gudar ska gå att bygga tempel åt nu?** A: de fyra minoiska gudarna som redan har böner (rekommenderas för första steget); B: alla kulturernas gudar som redan har fungerande böner (större katalog och fler namnval); C: hela pantheonet (många tempel skulle sakna fungerande böner). Poseidon i namneexemplet är inte en aktiv minoisk bön i nuvarande katalog.
2. **Vem är de gamla templen tillägnade?** A: en bestämd startgud per kultur, Potnia för minoer (rekommenderas om en omedelbar övergång önskas, men övriga kulturer behöver namnges); B: låt spelaren tillägna sitt gamla tempel en gång (eget val, men böner kräver val först). Det finns inget gammalt patronval att återställa.
3. **Ska en ny stad få ett gratis tempel när den grundas?** A: behåll att spelaren bygger sitt första tempel själv (rekommenderas i denna slice); B: välj patrongud och få L1-templet gratis, både huvudstad och koloni; C: gratis bara i första staden. B/C ändrar startkostnad och grundningsflöde. Ingen automatisk bemanning enligt13.
4. **Kräver någon av dagens böner ett större tempel?** A: alla dagens böner kan användas på L1 vid rätt guds tempel (rekommenderas först); B: välj uttryckligen vilka som ska kräva L2/L3. B gör vissa nu tillgängliga böner otillgängliga tills spelaren byggt ut.
5. **Ska varje tempel få ett eget dagligt offer?** A: varje tempel har egen offerkostnad från stadens gemensamma lager (rekommenderas enligt ett tempel=en lokal kult; fler tempel blir dyrare); B: en gåva per stad försörjer alla dess tempel (billigare att samla många). När lagret inte räcker: rekommendera synligt fast prioriteringsval, eller tills sådant finns en dokumenterad stabil ordning; aldrig flera utlovade gåvor med samma varor. Besluta också om kharis från arbetarna ska följa just deras tempels betalda offer (rekommenderas) eller dagens stads/rikesandel.
6. **Vad händer med ett erövrat tempel åt en annan kulturs gud?** A: byggnaden och guden står kvar, men bara tillåtna gudars böner går att använda (rekommenderad minsta regel; behöver avgöras om dess arbetare fortfarande bidrar till kharis); B: alla dess böner och kult får fortsätta; C: templet måste tillägnas en ny gud innan det används. Inget namnbyte eller ny kult sker tyst.

Redan givna beslut frågas inte om igen: flera tempel, högst ett per gud och stad; gudens namn på alla ytor; kharis gemensam för riket; samma arbetsstyrka och2platser/nivå enligt13. Rekommendationerna ovan är ännu inte kanon.

## Bevisplan för en framtida kodorder

Namngivet gammalkod-rött: två olika gudars tempel byggs och bemannas oberoende, samma gud stoppas även samtidigt, fel gud/nivå nekas av riktig HTTP. Egen mutation som låter valfritt tempel ge rätt bön måste bli röd. DB-prov för backfill/kö/retry/rollback, instansriktad rivning och ägarbyte; ingen dubbeldebitering när två tempel delar ett knappt lager; tick och forecast identiska. FOW visar bara behörig information. Webb/Keryx/Codex semantiskt samstämmiga; BILD vid fler tempelkort och långa namn, två rena scenarier före merge. Full färsk Go-svit en gång vid slutet, JS+vet enligt order. Dessa tester är planerade, **inte körda** under inventeringen.

## Grindutfall och stopp

Kod: ej tillämplig, ingen kod ändrad. Visuell/användare: ej körda, inga nya UI-anspråk. Semantisk: källanalys visar instans-, grundnings-, nivå- och offerluckorna ovan. Drift: ej tillämplig, ingen deploy eller liveoperation. Rapport och råa grep-underlag är leveransen. **STOPP för Claude/Timothys beslut före implementation.**
