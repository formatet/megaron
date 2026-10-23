# ORDER29 — begripligare kharistexter

Kontrakt före ändring: Haiku-testet fann motstridiga rivningsrader, otydlig knapphet/60-gräns och råd ovanför taket. Spelaren ska förstå omedelbar skuld, därefter löpande förlopp, och kunna förhandsvisa utan felmeddelande. Scope: CLI/webb/Codex-texter, tillhörande strängtester och fem acceptansutskrifter. Inga spelregler, API-nycklar eller regeltester ändras. Bekräftelse krävs fortfarande innan DELETE. Stoppa vid behov av ny regel. Bevis: strängbaslinje röd→grön, faktisk CLI i fem fixtures, JS, full fresh en gång och vet. Bas eb009264.

Knapphetskontroll: kharis/project.go scarcityFromValues använder templeOfferingRecipe (grain), divine/base klämd till 1–2. Därför säger texten referensoffret av korn, inte favoritvarorna för varje gud. Ingen regeländring.

## Resultat

Alla sju Haiku-fynd behandlade i CLI, Kult och Codex. Notisen om rivning använder samma uppdaterade texthelper i webb och CLI. API-fälten och domänreglerna är oförändrade. Förhandsvisningen utan --yes gör fortfarande aldrig DELETE, men returnerar nu lyckat med bekräftelseinstruktion; JSON svarar demolished:false, quote och message.

- Baslinjens uppdaterade CLI-strängtester röda (`baseline-cli.log`), därefter gröna (`targeted-cli.log`, `final-cli.log`). Inga domän-/regeltester ändrade.
- Fysisk mutation: borttagen spärr mot bemanningsråd ovanför taket → namngivet ORDER29 JS-test rött (`mutation.log`). Originalfil återställd, riktade tester gröna.
- 735 JS gröna (`full-js.log`). Två äldre strängförväntningar i Codex/notistest uppdaterades efter första fulla JS-körningen. Vet grön (`vet.log`).
- Fem riktiga Keryx-utskrifter: [SCENARIOS.md](SCENARIOS.md). Phoinix använder faktisk KharisTick; Thersites återanvänder den uttryckligen tillåtna konsistenta skyddsfixturen. Antilochos förhandsvisar utan rivning. Riggen står uppe. Klienten byggs från denna gren; servern är uttryckligen kvar på 7f0cdeb/migration203 eftersom serverregler inte ändras.
- Textens begriplighet behöver åter prövas av en spelare utan facit; automatiska strängtester visar innehållet, inte förståelsen. Inga nya balansbeslut eller visuella ändringar.

Full fresh migration203: grön, EN hel körning (`full-fresh.log`, 26 testade Go-paket). Efter att helkörningen startade togs en dubbel CLI-förklaring bort och två JS-strängförväntningar uppdaterades; slutlig CLI-paketsvit, 735 JS och vet kördes gröna för slutversionen. Ingen domänkod ändrades mellan kontrollerna. Kodcommits: 34331b82 + 3fb6670b.
