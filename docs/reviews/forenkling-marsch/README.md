# H — gemensam marschmeny

War krävde en separat Q/R-panel och kartmenyn började med noll enheter. Nu väljer
War → March just den enheten och ett vanligt kartklick öppnar samma meny som
högerklick. Alla lediga i första gruppen är förvalda. More håller stance,
expeditionslängd och koloninamn; lastad galär kan landsätta sin enhet.

Bas: `codex/recall-all-enkel` @ba96c79b. Produktionskod senast **b1425e0f**;
slutlig beviscommit läses ur grenens HEAD. Bevisar geografikedjans nåbara orderyta.
Ingen merge/push/deploy. **BILD: Claude beslutar** enligt Timothys delegation
2026-10-07; ingen väntan på Timothy. Claude beordrade därefter en slice åt gången:
överlämna H och stanna; I–O har inte startats.

| Kontrakt | Innehåll |
|---|---|
| Spelarsanning | Förvald första grupp, exakt vald War-enhet, kartmål även med vanligt mobilklick. |
| Invariant | Samma per-enhets-POST/body, FOW och serverregler; explore, redirect, stance, kolonisering och kolonilast bevaras. |
| Scope | Gemensam marschmeny, War-ingång, kartans målklick, Codextext, gemensam prosaformatterare och provverktyg. |
| Non-scope | Server, CLI, kanon, War-kortets övriga handlingar (I), nya CSS-klasser/färger. |
| Acceptans | Alla i första gruppen/övriga noll; More stängt; War låser enhet; lastad galär landsätter; mobilmenyn ryms. |
| Stopvillkor | Regeländringar till Claude; dessa behövdes inte. |
| Bevisplan | Baslinje, 1:1 desktop/390px, JS-kontrakt, fysisk mutation, färska webbvärldar med kvitto + read-only audit. |

| Grind | Bevis |
|---|---|
| Baslinje | `live-baseline`: health11246fb1/mig160, oförändrade ba96c79b-assets; två manuellt valda enheter, två audits, båda hem. |
| Kod | **390 JS gröna**. Full färsk PG16/mig160 Go-svit @981d2411 grön (`logs/go.log`, world86.160s), full vet grön före checkpoint. Därefter endast UI/text/prov; **serverdiff mot basen tom**. |
| Mutation | quantity, pin och cargo_intent fysiskt brutna → varsin namngiven assertion röd → återställd grön. `mutations/`. |
| Användare/semantik | Tre separata färska slutriggar: **healthb1425e0f/mig160**. Map två förvalda enheter → två audits → båda hemma; War exakt vald enhet via vanligt klick → en audit → hemma; land en lastad galär → en audit → lasten positioned på rätt mål och galären hemma. Samtliga utan browserfel/SQL-mutation/menyoverflow. Map/War och landing har även föregående färsk grön genomkörning. |
| BILD | `live-baseline` före; `live-map`, `live-war`, `live-land` efter. Desktop/390px vid 1:1. Granskas av Claude; ingen egen estetisk godkännandestämpel. |
| Drift | Ej deployad; egna riggprocesser och containers stängs av verktyget. |

Landprovets gamla fel var **River (6,37)**, inte ägt catchmentland: servern räknar
river/river_ford som vatten. Riggen väljer nu bart känt kustland och en verklig
kustgrundning med Poseidons galär via vanliga spelaranrop. En första återstart
hade inlandsspawn utan galär; det var också en provförutsättning, ingen regression.
Bilder väntar nu på färdigt ETA och pekaren flyttas normalt för att stänga hovertext.

Avgränsning: kolonilastens `cargo_intent=colonize` + namn har kontraktsprov och
mutation; ingen genomförd koloni via galär hävdas. Den tidigare separata War-panelen
hade ingen kolonilastförhandsvisning; samma serverkvitto/purse visas fortfarande.
Gruppens medlemsnumrering, koordinater och redigerbara fält är numeriska; spelarprosa
för mängd/dygn/prognos använder ord. War-kortets kvarvarande jargong tillhör I.

Reproduktion: [tools/README.md](../../../tools/README.md#gemensam-marschmeny-h).
Kör `node --test 'web/static/js/megaron/**/*.test.mjs'` och
`python3 tools/simple_march_mutations.py OUT` från detta worktree. Bygg rena
server/keryx-binärer med produktionens hash, kör sedan `simple_march_live.py`
i separata OUT för `map`, `war` och `land`; varje arm skapar tom DB/Redis själv.
Checkpoint/paus @8692afad är historiskt; slutrapporten ovan ersätter dess Resume.
