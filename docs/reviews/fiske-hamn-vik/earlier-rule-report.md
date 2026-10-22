# ORDER 14 — hamn och vik styr fiskplatser

## Sammanfattning

Djuphav kräver hamn; en kustvik med minst fyra landsidor har en extra plats.
Kodcommit `245f18bd`, bas `ea97b332`, migration198 (ingen ny migration).
Gren `codex/fiske-hamn-vik`. Ingen merge/push/deploy eller livedataändring.
Inventering, SQL och historik: [inventory.md](inventory.md). Slutlig full fresh Go GREEN (world153.336s),714JS/vet GREEN, namngiven RED och fysisk mutation verifierade.

## Slice-kontrakt

Spelaren bemannar de fiskplatser servern faktiskt använder. Kust2/6, vik3/7,
djuphav0/4 utan/med hamn. Alla hamnnivåer har samma antal; rate multipliceras
med1+0.7×nivå även på djuphav. Flod/vad oförändrade. Land utesluter bara
sea/coastal_sea/deep_sea; river/ford räknas, saknad kartgranne räknas inte.
Ingen ny hamnbyggplats, ingen ändrad catchment eller informationsdelning.
Stopp vid nytt kanon-/övergångsbeslut; Claude godkände land/FOW/deepökning före kod.

## Baslinje och regelägare

`baseline-red.log`: TestDeepFishingRequiresHarbour körd mot gammal produktion
på egen färsk mig198-databas; gammalkod gav1plats på alla nivåer istället0/4.

`loadHexRuleTiles` i economy/hex_rules.go äger kartkontexten och räknar
kustens landgrannar med hexgrid.Neighbors-offsets. Produktionsoptions,
LoadHexCapacity, HexBuildEffects och RuleBuildingGoods konsumerar dessa regler.
Geometri är delad; grannterränger eller grannantal skickas inte till klienten.
Råtabellens terräng-/byggnadsgrindar känner vilka varor byggnaden hjälper;
ChooseFoundingFarmHex väljer fortsatt endast grain/farm och behöver ingen vikdata.

HTTP buildGoods utelämnade tidigare cap0 som null, vilket skulle kunna visas som
obegränsat. Nu serialiseras en närvarande kapacitetsnyckel även när värdet är0.
PlaceGubbes befintliga cap<=0-avslag används. Ingen ny endpoint/klientformel.

## Experiment och grindar

| Prövning | Resultat/bevis |
|---|---|
| Gammal kod | Named RED i baseline-red.log |
| Kontextägare | Vik>=4, river/ford, kartkant; options/cap/effects/goods delar ägare |
| Livscykel | TestFishingOwnerOptionsCapacityEffectsAndDemolition:18deephexar0/4, L1–L3 rate, faktisk placerad L3-produktion3.1/6, hamnrivning frigör fiskaren |
| Grundning/restfolk | Nollkapacitet ger inga grundningsslots/restfolkfisk i samma owner-test |
| HTTP | TestDeepFishingHTTPZeroCapAndHarbour: synligcap0,409utanhamn→cap4/201medhamn,1.7/6output; TestBayFishingHTTPUsesLocalCapacity:L0cap3,L1–3cap7,inga grannfält |
| Egen mutation | Separat /tmp-worktree på245f18bd, land>=4→>=5; owner-DB och HTTP-test RED; byteexakt SHA-restore och ny färsk GREEN. mutation-*.log/txt |
| Klienter | Befintliga Keryx/webb läser servercap/rate. 714JS gröna inklusive nollcap och2/3/4/6/7. Se client-surfaces.md |
| Vet | go vet ./... GREEN, vet.log |
| BILD | Ingen produktions-JS/CSS/markup ändrad. Befintlig hexpaneltext är oförändrad; ingen BILD-grind utlöses. Artikeltext i playtest |
| Full fresh Go | GREEN, world153.336s; full-fresh.log, EN körning på57900 |

Detta är riktiga PostgreSQL-/HTTP-tester och befintlig klientrendering med
explicit fixtures. Ingen manuell spelarrunda eller ny Firefox-bildserie påstås.
Targeted/economy-loggar är tidigare färska delkörningar; full-fresh är helsviten.
En mellanliggande riktad körning stoppade på felstavat testhjälparnamn
(optionsPath); det rättades till befintligt placementOptionsPath före gröna bevis.
Den slutliga targeted.log ersätter delkörningens kompileringslogg; råutfallet
återges här och tillskrivs inget produktionsfel.

## Drift och begränsningar

Kör live-readonly.sql och automatic-readonly.sql DIREKT före deploy och jämför
med inventeringen. Kommando från grenroten:

```sh
ssh root@10.0.1.92 'runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d poleia' < docs/reviews/fiske-hamn-vik/live-readonly.sql
ssh root@10.0.1.92 'runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d poleia' < docs/reviews/fiske-hamn-vik/automatic-readonly.sql
```

Ingen snapshot är reservation. Om fiskare utan hamn tillkommit behövs uttryckligt
övergångsbeslut före deploy. Ingen automatisk omplacering/backfill införs här.
Ordinarie Kharistick anropar RecomputeProduction och uppdaterar sparade rates;
API-options visar regeln direkt. Ingen generell extra uppstartsrekalkyl införs
som också skulle kunna köra andra stockrecept. Claude har fått denna skillnad
för samordning med ORDER13s uppstart. Ny regel gäller framåt; inga historiska
matlager räknas om. Djuphavets grundrate1/6 och kust1 är oförändrade.

ORDER13 och17 är ännu separata grenar; denna gren är byggd påea97b332.
Integrationsordning/rebase och eventuell ny kontroll efter merge ägs av Claude.
