# ORDER30 — marknadens arbetsplatser bort

Kontrakt före kod. Bas `6694e776`, gren `codex/marknad-platser-bort`. Timothys beslut i temenos_arbete: marknaden har inga arbetsplatser. Befintliga placeringar ska bli fria gubbar utan befolkningsförlust. Marknadsbyggnaden och handeln består; nivåer, kostnader och handelsregler ändras inte.

Inventering FÖRE ändring: live read-only på migration204 (raw `live-inventory.json`) visar **0 marknadsplaceringar**, **10 marknader** (4 L1,4 L2,2 L3), en market→pottery-regel med rate≈1 men varan är parkerad. Tabellen ger ändå1/2/4 platser. API/CLI/webb bygger arbetsplatsmenyn från produktion/kapacitet; den parkerade varan gör att menyn redan saknar marknaden, men restregeln skulle återaktivera den vid senare varustatusändring.

Scope: ta bort kapacitet och produktionsregel, frigöra gamla fysiska placeringar atomiskt, hindra nya marknadsplaceringar och ge samma besked på server/Keryx/webb/Codex. Invariant: population och andra placeringar/varulager bevaras. Acceptans: noll kapacitet alla nivåer; även aktiverad pottery ger ingen marknadsmeny; gammal placerad gubbe blir fri; andra jobb består; marktilldelning nekas men byggnaden finns kvar. Stoppa vid behov av ny handels- eller nivåregel.

Bevisplan: namngiven röd kapacitetsbaslinje, riktig PG205 med gammal placement/rule-fixtur över migrationen, API/CLI/webb-paritet, egen fysisk mutant och EN full fresh vid slutet + JS/vet. Två rena spelaringångar för återlämnad arbetskraft och nekad ny placering.

## Resultat och fyra ytor

Kod `0f32a5fe` (klient/Codex `8b8aafb3`), migration **205 up/down**, bas6694e776. Marknaden är kvar i bygglistan med beskedet ”no workers needed”. Marknadsnivåer och handelsregler är oförändrade.

- **Server:** kapacitet0 alla nivåer; market→pottery-raden bort. Gamla market-placeringar raderas atomiskt och blir idle genom den befintliga workforce-modellen. Ingen population, annan placering, byggnad eller lagrad vara ändras. API vägrar ny marknadsplacering; producentmenyn utesluter market även om någon senare aktiverar pottery och råkar återinföra en regel.
- **Keryx:** `staff market` förklarar inga arbetsplatser/inga arbetare, även JSON. +n/−n vägras före HTTP-anrop. Körda faktiska kommandon i `cli-market.txt`/`cli-refusal.txt`.
- **Webb:** generiska gridden läser riktiga placement-options och visar ingen marknadsarbetsplats. Renderaren är oförändrad; nya testet lägger inte till ett lokalt filter. Faktiska API-options renderades av samma `cityBuildingsHTML` i två acceptansfall (`MarketProof30*-web-workplaces.html`). Inga nya pixel-/layoutändringar att bedöma som BILD.
- **Codex:** buildings och trade förklarar att marknaden inte behöver arbetskraft och att tidigare anställda är fria. Inga nya handelsfunktioner lovas.

## Bevis

`TestMarketHasNoWorkplacesAtAnyLevel`: oförändrad kapacitet **RÖD** (`baseline.log`), riktad fresh205 **GRÖN** (`targeted.log`). Migrationstestet kör gammal worker+regel genom down/up i rollback-transaktion, två separata fixturestäder; samma500personer, matodlaren kvar, marknad kvar, gamla arbetaren fri. API-försök tar ingen medborgare; nästa riktiga grain-placering använder den fria ordinalen.

Fysisk mutant tog bort just migrationens DELETE av gamla placeringar: `TestMarketWorkplacesMigrationFreesWorkersWithoutLosingPeople` **RÖD**, en gammal arbetare blev kvar (`mutation.log`). Byteexakt återställning (`mutation-restore.sha256`), samma test **GRÖNT**. Down återställer de exakta gamla produktionsraderna från en snapshots-tabell, men återplacerar aldrig en person som kan ha fått ett annat jobb sedan dess.

Acceptansservern uppgraderades faktiskt från204 till205 med två dedikerade gamla marknadsplaceringar; `/healthz`-provenance i `provenance.txt`. Första provet korsade ett vanligt mattick: fixturen saknade mat och population600→597 (se `acceptance-initial-fixture-failure.log`); det var ett fixturfel, inte ett rent migrationsbevis. Teststäderna fick därefter mat, och **exakt205 down/up-SQL** spelades om atomiskt i den isolerade205-riggen, utan ändring av schema_migrations. Båda:600personer, samma matodlare/marknad/1000silver, idle4→5. API422, faktisk Keryx-förklaring och verklig webb-renderare stämmer (`acceptance-before.json`, `acceptance-after.json`, `acceptance-check.log`). Övriga konton och live har inte skrivits.

EN full fresh migration205, 26 Go-paket **GRÖNA**, world154.925s (`full-fresh.log`). 738 JS och vet **GRÖNA**. Ingen balans- eller designändring utanför beslutet; ingen merge/push/production-deploy av Codex.
