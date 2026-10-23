# Nå den redan tillåtna offerkompositionen

Kontrakt: en Wanax som har tempel, tillräcklig kharis och ingen aktiv/väntande bön ska kunna öppna offertberedningen även utan standardreceptets varor. En rökelsegåva som rite-handlern redan godtar måste gå att lämna genom webbens normala flöde. Invariant: öppet formulär lovar inte betalningsförmåga; lager/värde/krav kontrolleras fortsatt atomiskt av samma Rite. Scope: serverägd can_prepare_offering i läsytan, webbens förberedelse och tydligt tomt val, Keryx-lista, Codex och bevis. Inte scope: traditionella affordable-fältets betydelse, ritens regler, värdering, chance, recept eller fler varor i det befintliga UI-urvalet. Stoppvillkor: behov att ändra Rite-betalningsregler eller prioriterade kanonkrav.

Beslutstöd: temenos_prayers_komposition_plan och dagens rite_relation-prover tillåter andra tillräckligt värdefulla varor. R1/R4 ändrar inte detta: dagens vardagsböner svarar när kraven uppfylls, kharis är en tröskel. Baslinjen ska visa faktisk godkänd incense-POST samtidigt som läsytan saknar åtkomst till formuläret. Bevisplan: rött→grönt API/JS/CLI, nya flaggan aldrig sann vid tempel/kharis/wait/aktiv/pending-spärr, tom komposition gör inget POST/fallbackoffer, source-mutanter, två färska verkliga Firefox/Keryx-flöden och fullfresh en gång.

## Baslinje och riktad verifiering

På gammal kod godtog en verklig komponerad incense5-POST gåvan utan grain/silver, men tillståndsytan gav ingen möjlighet att bereda den; baseline-api.log. Webbens förberedelselänk och tomma alternativfält samt CLI-listans compose-status var också röda på gamla konsumenter (baseline-js.log/baseline-cli.log).

Nu bär läsytan can_prepare_offering från tempel/kharis/tidskrav, separat från standardreceptets affordable. Förberedelsen lovar varken lager eller tillräckligt värde. Inga ändringar i Rite, reserveringar, offertröskel, priser, väntan eller kharis. Nuvarande och äldre tidsgrindar nollställer flaggan; sju riktiga HTTP-scenarier skyddar saknat tempel, låg kharis, godwait, aktiv gåva, pending och redo/alternativt offer. Befintliga väntetextprov fortsatt gröna.

Webb: Compose offering öppnar nollställda fält när standardgåvan saknas. Tomt val gör ingen POST och faller inte tillbaka på standardreceptet. Positiva val skickas genom samma befintliga castRite. Keryx-listan visar compose och hänvisar till redan befintliga --offer. Codex rites beskriver vägen.

Fysiska källmutanter: serverns förberedelseflagga alltidfalse, webbens nya åtkomst avstängd och CLI:s JSON-nyckel felstavad. Alla gav namngivet rött; byte-exakt återställda i finally. Riktade prov gröna efteråt. 742 JS-prov och go vet ./... gröna. Full Go-svit körd EN gång mot färsk migration205:26 paket gröna, world154.819s.

## Verkliga klienter

Run2 och run3 använder två egna nya PG16/Redis-runtimes och normal register/join/found. Därefter en explicit egen tempel-/stock-/fyndfixtur: bara incense100 i staden, grain/silver0; inga SQL-skrivningar under handlingarna. Det bevisar UI/API/affärstransaktionen, inte naturlig insamling av rökelse.

Run2: native Firefox öppnar Compose offering, alla fält är0, ett tomt Bring the offering ger lokalt fel och0rite-POST. Incense5 fylls i och lämnas via riktig POST. Run3 gör samma tomvalsprov i webb och lämnar sedan incense5 med faktisk nybyggd Keryx rite --offer/--yes/--json. I båda återstår incense95 och grain/silver0; kartan får bara minne, och efteråt visar Firefox1280/390 plus verklig Keryx-lista24dagars väntan utan ny offerknapp.0pageerrors/ingen sidledsöverfyllnad. Egna processer/containrar städade i finally. Process124815f+compose/migration205 och käll-diffhash sparade. run3/proof.json anger cast_via=cli, cast-cli.json bär den verkliga kvittensen.

Run1 stoppades av verktygets case-sensitive knapptextkontroll: CSS gör Compose offering till versaler i innerText. Kontroll korrigerad till case-insensitive; detta var inte ett produktfel. Dess logg är bevarad. Godkända efterkörningar är run2/run3.

Mellan run2 och run3 tillkom endast förklarande källkommentarer och provets cast_via-metadata; körd produktlogik är densamma. Käll-diffhasharna bevaras var för sig.

## Levererat

92d183c0 mergad/pushad och NY liveprocess92d183c/migration205/ok verifierad. Grund:205 dirty=false och samma aktiva värld2178e795. Levererade Kult/offer/Codex-filer byte-exakta; SHA i deploy.json. Inga ERROR/fatal/panic i granskad journalperiod. Installerad Keryx vcs.revision92d183c0cf0021e83cdd790a3f675dc8601806f1, vcs.modified=false. Ingen migration/reseed och ingen ändring av Rite-betalning eller krav.
