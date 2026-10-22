# ORDER 7 — Wants bort

Kontrakt före kod (bas4054bd63): problemet är en stående lista med andra städers brist/överskott. Spelarsanningen är att fråga Wanax genom befintligt brev/korrespondens. Webb får tre Economy-flikar; Keryx-kommandot och API-routen tas bort. Exklusiva handler-/CLI-tunables rivs; market_snapshot.go, visibleOrigins och handelsmekanik behålls. Acceptans: route404/405, CLI okänt kommando, tre flikar, inga döda Codex-länkar. Funktion i prov; TEXT i playtest, ingen BILD. Bevisar lika informationsvillkor i geografi→brist→brons→elit. Ingen livevärld eller flottkonfiguration ändras; Claude äger fleet-whitelist/prompt.


## Resultat
Wants saknas på webb, Keryx och API. Economy har Goods/Transfer/Automation. `MarketWants` och dess gamla stock/rate-prov, enbart dess tre discovery-tunables samt Keryx-kommandot och dess exklusiva rateStr-hjälpare är borta. Ägarnas referenser söktes före rivning. Det gamla kommandots positiva prov ersattes med ett negativt prov av riktiga root-command: `wants` och `wants --json` är okända, kommandot saknas i hjälp/alias.

Delad `economy/market_snapshot.go`, `visibleOrigins`, trade offers och kontaktskrivning är orörda. Ingen migration eller eventändring. Snapshotfiler har inte raderats bara för att den stående läsytan försvinner. Verbbibliotekets codex_aliases-rad för gamla GET-routen togs bort; Umami-registret hade ingen sådan GET och är byteexakt oförändrat (`shared-owners-unchanged.json`). Paritetsinventeringen innehåller inte längre routen (`parity-summary.json`); generella äldre paritetsluckor ingår inte i denna slice.

Sex Codex-artiklar pekar på att fråga Wanax i brev genom befintlig Correspondence/`keryx messenger` och läsa svaret i inbox. Inkommande förhandlade trade offers kan fortfarande säga “Wants 40 tin” och generisk engelska “someone who wants it” är kvar; detta är inte den borttagna marknadslistan. `codex.test.mjs` verifierar länkarna. Vault `megaron_verblista.md` stryker wants med datum2026-10-22 och Timothys beslut. Todo/övrig canon synkas av Claude.

## Bevis och grindar
- Före ändring: riktade gamla Wants API/Keryx-prov GREEN på ny PG16/mig198 (`baseline.log`).
- Efter ändring: `GOTEST_PORT=57900 GOTEST_KEEP=1 tools/gotest.sh` en gång, ny PG16/mig198, hela Go-sviten GREEN; world153.658s (`full-fresh-go.log`). `go -C server vet ./...` GREEN.705/705 JS GREEN, inklusive tre Economy-flikar och frånvaro av både panel och GET-anrop (`js-summary.log`).
- Fysisk mutation återinsätter Wants-knappen: namngivna `drawer keeps three tabs` blir RED; originalbytes återställda → GREEN (`mutation-red.log`, `restored-green.log`).
- Faktiskt byggd server kördes lokalt på en separat ny databas/schema198 och egen tillfällig Redis. `/healthz` visade `4054bd6+dirty`/mig198, `/worlds` och autentiserad `/auth/me` svarade200. Både GET och POST på gamla `/market/wants` svarade404 utan och med giltig Bearer. Byggd Keryx-binär gav exit1/unknown command. `runtime-proof.json`.
- Ingen BILD enligt order; TEXT i playtest. Inga livekonton, delade acceptansvolymer eller AI-spelare användes i proven.

Runtime-recept: bygg `go -C server build -o /tmp/megaron-wants-proof/server ./cmd/server` och motsvarande `keryx`; kör `python3 docs/reviews/wants-bort/runtime_check.py <den egna behållna megaron-gotest-PID-containern>`. Skriptet skapar/raderar en egen databas i denna container och egen Redis, startar server med explicit egna env/template/static-sökvägar och stänger processen efteråt. SQL-hjälpare i själva Go-testdatabasen kan ha ersatts av tester; runtimeprovet använder därför en separat, ny migrerad databas. Den behållna gotest-containern städas av utföraren efter provningen.

SHA256 för ändrade produktions-/testfiler, inklusive borttagna filer, finns i `tested-files.json`. Ingen produktionskod ändrades efter helsvitstart; den fysiska webbmuten kördes efter helsviten och återställde identiska bytes före runtimeprovet. Server-/Keryx-binärerna byggdes från samma kod. Ingen merge/push/deploy.

## Överlämning som krävs i flottan
**Claude måste ändra** `~/.local/share/megaron-luna/luna_command.py`: `ALLOWED` listar fortfarande `wants`. Uppdatera även flottans prompt så den frågar med brev. Dessa runtimefiler har uttryckligen lämnats åt Claude och är inte ändrade av Codex.
