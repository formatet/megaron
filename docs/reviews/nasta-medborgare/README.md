# Next citizen proof

Run `python3 tools/nasta_medborgare_bild_20261024.py docs/reviews/nasta-medborgare/browser`.
The script owns fresh PostgreSQL, Redis and server processes and cleans them up.
It builds Keryx too. No shared acceptance or production environment is used.

A genuinely founded city is given explicit population, wall, stocks/rates and last
meal fixtures. Growing has1964people,wall1,positive grain production; holding has
1964people,wall1,zero production and no unmet meal; capped has2000people,wall0;
shrinking has1964people and food_unmet_amount1. The world is tick100 at1800seconds
per tick. SQL is recorded. Every frame follows an actualAPIread and normal City
drawer load, with no mocked responses or replaced DOM. The server supplies the
progress and forecast, which the script checks before using the real renderer.

Each state and viewport also runs the compiled Keryx `city` and `status` commands
against this world. Their captured outputs are linked from the gallery.

The selected final run records its build health and source hashes in proof.json.

## Resultat

Produktionscheckpoint **5dc321f7**, server byggd från denna commit, migration **210**.
Ingen migration tillkommer. `economy.PopulationGrowthSQL` är den gemensamma ägaren
för `applyDecay` och `LoadNextCitizen`. API skickar progress, faktisk förväntad
bruttotillväxt och nullable uppskattning. UI och Keryx räknar ingen tillväxtformel.

Under Citizens visas tio hela CSS-steg och resten av hundratalet. Den aktuella
världens tempo kommer via `fmtDuration`/motsvarande Keryx-formattering; inget
tempo är hårdkodat. Stillastående, murfull och svältande stad saknar påhittad ETA.
Codex använder den befintliga artikeln `growth.md`.

## Bevis och grindar

- **Rött först:** `TestNextCitizenStatusMatchesActualGrowthTick` fann saknade
  API-fält på gammal kod. Slutprovet kör autentiserad GET och riktig
  `kharis.TickHandler.Handle`, med oberoende förväntade värden i tio lägen:
  variation, half-even-avrundning, minsta tillskott, klämning vid tak,
  vid/över tak, inget matöverskott, svält, gammal måltid och obetydlig vinandel.
- **Tre fysiska ytmutanter:** borttagen variation (8→6), minsta tillskott
  borttaget (1→0), murklämning borttagen (1→10): alla RÖDA vid rätt assertion.
  Exakt källåterställning; berörda economy/kharis/API-prov därefter GRÖNA.
- **Klientgrindar:** 763 JavaScriptprov gröna inklusive Codex; hela Keryx-paketet
  grönt; `go vet ./...` och fyra day_guard gröna.
- **Användar-/bildbevis:** [galleri](index.html), åtta native Firefoxbilder
  1280/390 över växande, stillastående, murfull och svältande stad. Alla granskade
  1:1, noll sidfel. Riktig API och Keryx `city` + `status` i varje fall
  (16 kommandoresultat). 932 produktionshashar matchar slutkoden; samtliga
  gallerilänkar fungerar. Färsk egen PG/Redis/server-rigg, städad efteråt.
- Explicit fixtur: genuint grundad stad, sparad population/måltid/produktion
  för respektive tillstånd. Ingen mockad respons eller ersatt DOM.

Full Go-svit körd en gång på färsk migration 210 från 5dc321f7: alla 26 testpaket GRÖNA, inklusive world 150,036 s. Ingen skip eller omkörning.
Ingen prod/acc, merge, push eller deploy. Varvet och truppslicen är orörda.
