# Game days

Kontrakt: `.agents/order-game-days-20261024.md`, basfd7b6450. Blockerar begripligheten i kedjegrinden: längd/takt ska inte läsas som verkliga dygn. Spelaren möter game day i mat, produktion, sold, byggande, böner och resor.

Alla fyra ytor använder game day(s) för längd/takt. Detaljer, förhandsvisning, byggkvittens och skydd får väggklockestöd; trånga takt-/armé-/varurader har bara game day. Ingen migration, ändrad ticklängd eller annan klockregel, inga gamla event omskrivna. Ingen merge/push/deploy. Stopvillkor: behov av ny tidsmekanik eller migrering.

Bevisplan: vänd guard först och spara RED mot gamla spelarytor, fyra fysiska språkmutationer, dynamiska1800/3600/6-sekundersprov, färska Go-paket och en hel färskGo-svit, JS/Codex/vet, egen Firefox/Keryx-rigg. Kalendernamn och tekniska datafält bevaras.

## Inventering och genomförande

[Inventeringen](inventory.json) innehåller bruttot49webblitteraler/16filer,62Keryx/16filer,129Codexrader/40artiklar och32serverlitteraler/10filer. Bruttot inkluderar kalender, redan explicita game days och intern diagnostik — det är inga påståenden om272spelarbuggar. Baslinjens fyra guardloggar visar exakt vad den vända regeln fällde före svepet.

Webbens `State.TICK_SECONDS` kommer från world-API och klientens tickankare. Keryx `Client.TickSeconds()` läser samma `tick_seconds`-kontrakt. `fmtDays`/`formatDays` äger singular/plural/qualifier, `fmtDuration`/`formatDuration` lägger till faktisk väggklockelängd i detalj/preview.1800/3600/6-sekundersprov ger olika rätta vägglängder; saknad kadens ger bara game days. Täta produktion-/soldrader får ingen väggklockeomräkning av takten.

ByggPOST:s redan beräknade `complete_tick` och `spec.DurationTicks` exponeras som `complete_tick`/`duration_game_days`, så webben och Keryx kan visa samma accepterade längd + klart. Ingen ny tidsberäkning eller migration i servern. Public alpha-info använder också runtimekadensen i stället för ett statiskt “half hour”. Befintliga ankomst- och slutklockslag fortsätter använda tickankaret; kalenderheadern “Day…of…Year…” och Shadow Days är orörda. Gammal pickuptext med absolut dagnummer ersatt med klockslag; gamla sparade notiser har inte ändrats.

`day_guard.py` tillåter game day(s), inklusive ungefärliga längder, och fäller nakna day(s), ticks och gamla turn-uttryck. Kalendern undantas genom grammatik, inte radnummer. A.5-undantagen för game day är rivna. Tekniska JSON-/mapnycklar bevaras. `gossip.go`'s `3 days` är ett PostgreSQL-intervall för recency, inte spelartext, och har ett uttryckligt infrastruktursundantag; mekaniken är orörd. “Decide the day” är ett idiom och skrivs “decide the battle”.

## Experiment och bevis

- Vänd guard **RED mot gammal produktion**: `baseline-{web,keryx,codex,server}-red.log`.
- Fyra fysiska språkmutationer **RED→exakta bytes→GREEN**, en per yta: `mutations/results.json`; reproduceras med `tools/game_days_mutations.py`.
- Guardens tre tester och alla fyra språkytor gröna: `guard-tests.log`, `guards-green.log`. Kalender + längd i samma mening testas så kalenderundantaget inte döljer ett naket “3 days”.
- Riktade färska Go-paket: API/combat/tick/religion/gossip gröna i `affected.log`; äldre CLI-förväntningar i den körningen behövde även vägglängden och skyddstexten. Därefter `keryx-final.log` och `final-targeted.log` gröna, inklusive serverns publika alpha-sida och runtime1800/3600/6.
- JS och Codex: `js-final.log`, `codex.log`. Tidiga JS-loggar visar gamla ord-/regexförväntningar, inte kvarvarande produktionsfel. Vet: `vet.log`.
- [BILD](index.html): två oberoende nyskapade PG/Redis/server-världar, ticksekunder1800 respektive3600, riktig Firefox1280/390. Normal registrering/join/grundning föregår sparad lokal bygg-/varufixtur. Samma stall:8game days→4/8realhours. Faktiska browserPOST201 och CLIbyggen visar game days+realtid+klart;0pageerrors. Källhash före/efter varje lopp och slutkontroll i `final-source-verification.json`.
- Första riggförsöket valde en redan maxad kasern (`browser-max-level-fixture-error.log`); korrigerat fixturval. Nästa lopp hittade ett **verkligt kvittensfel**: `refreshCityBuildings` ersatte textnoden direkt efter201 (`browser-receipt-red.log`). Den accepterade kvittensen återställs nu efter refresh, endast om samma värld/stad fortfarande visas. Båda slutliga världsloppen är gröna efter fixen.

## Grindar och avgränsning

Ingen ticklängd, schemaläggning, skyddstid, livscykel eller DB-balansregel ändrad. API-fälten är additiva; råa JSON-/CLIflaggnamn behålls. Historiska eventtexter och interna SQL-intervall översätts inte. Alla egna riggar städas i finally; delad megaron-acc-rigg och produktion orörda. BILD är bifogad inför Claudes integration; textens begriplighet kan fortsatt bedömas i playtest. Nästa BILD-order är inte påbörjad.

Slutresultat: hela Go-sviten **en gång** på nyskapad PG16/migration210,26testpaket GREEN/world152.207s (`full-go.log`);757JS/14Codex/vet/fyra språkgrindar/tre guardtester GREEN. Go-källan var frusen under fullsviten. Efter dess start ändrades endast webbens kvarhållna byggkvittens; JS-sviten och båda slutliga nativevärldar kördes efter den fixen. Slutligt910produktionsfiler per nativekörning matchar oförändrad slutkälla. Branchens hash finns i gitlogg/agentöverlämning; ingen rebase under proven.
