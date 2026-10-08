# R — recall från senare expeditionsben

Kontrakt före kod, bas 35232e3d: när Recall når en marscherande expedition ska
återfärden gå till dess home_settlement_id, inte det aktuella benets starthex.
Använd expeditionens befintliga val av ägd, aktiv och nåbar hemstad (med samma
fallback om hemstaden förlorats). Saknas nåbar hemstad avvisas ordern och den
pågående kursen kvarstår. Expeditionen avslutas vid recall; ingen fullföljd
expeditionsrapport. Återfärden ska sluta i garnison, även när ett skepp anländer
på hemstadens havshex. Vanlig marsch-recall och explicit redirect behåller sin
befintliga destination och avsikt. Runner-leverans och fart/rutt/tickregler består.

Bevis: verklig StartMarch → första ankomst → senare ben → ExecuteRecall →
UnitArrival → garnison; röd före ändringen, grön efter. Färska spelvärldar via
webb som i P, vanlig marsch och redirect-regression, full färsk Go, vet och
mutation av destination respektive återkomstavsikt. Fyra ytor inventeras.
Ingen ny spelregel, layout eller notifiering; detta reparerar expeditionens
befintliga hemkomstlöfte och blockerar korrekt avslut av spelarens utforskning.

## Resultat

Produktionskod och nya regressioner: `c7e8496c6a12951314432eee89e5be91bab7d44a`.
Endast combat-kärnans destinationsval och returavsikt ändras. Samma route,
fart, injected clock, Runner-kuvert och ankomsthändelsetyper används. Befintlig
`home_settlement_id` läses under enhetslåset och eventuell fallback sparas i
samma transaktion som det nya benet; `unit_expeditions` raderas som förut.
`explore_return` når befintlig `exploreReturned`, som återgarnisonerar och
rensar hemstad/avsikt. Återförsök av samma arrival ger ett enda utfall.

- `baseline-combat.log`: oförändrad bas grön på färsk migration 160.
- `red.log`: riktiga StartMarch/arrival-livscykler röda land och naval på senare
  bens starthex (4,0); vanlig recall och redirect gröna redan på basen.
- `green.log`: senare-ben-recall → hemgarnison, återförsök, avbruten mission utan
  report, naval havsgranne, vanlig recall, redirect med bibehållen Explore-avsikt,
  förlorad hemstad med fallback och avslag utan mutation om ingen hemstad finns.
- `full-go.log`: **hela** Go-sviten grön, ny tom PG16/ren migration 160,
  `env -i`, `-count=1 -p 1`. `vet.log` tom och exit 0. `js.log`: **433/433**.
- `mutation-destination.log`: återställ destination till origin → named röd.
  `mutation-arrival-intent.log`: ta bort returavsikten → named röd, kvarvarande
  home_settlement_id upptäcks även om hexuppslag råkar ge garrison.
  Produktion återställd före fullsviten.
- `live-baseline`: ny spelvärld, register/join/grundning, riktiga War-klick,
  expeditionens senare returben, Runner202 och levererad `MarchRecalled`:
  hem (53,10), fel target (54,6). Avsiktlig röd på destination, före ankomst.
- `live-after1`: samma spelarflöde; benstart (45,11), hem (47,7), levererad
  recall target (47,7), slut `garrison` i den grundade hemstaden.
- `live-after2`: benstart (9,57), hem (12,53), levererad recall target (12,53),
  slut `garrison` i den grundade hemstaden. Båda har 3 stance-order, en march,
  en recall, inga browserfel/OrderDeliveryFailed eller SQL-ingrepp.
- `live-mutation-destination`: separat kopia av serverkällan, en mutation,
  separat binär och ytterligare ny värld. Samma webbaserade assertion blir röd:
  hem (13,57), fel target (10,61). Hälsan märks uttryckligen mutation-destination.
  De två gröna armarna använder identiska rena binärer; sha256 i manifestet.

De fysiska expeditionsproven fångar ett senare **returben**. Go-receptet fångar
ett senare **utgående** ben; båda tidigare felaktiga fallen är därmed täckta.
Alla bilder är befintlig War-layout, desktop/mobil; ingen layout ändras i R.

## Fyra ytor och gränser

Temenos delar ExecuteRecall mellan API:s personliga order och Runner-leverans.
Keryx `requestRecall` och webb `unitRecall` skickar samma tomma kropp till samma
rutt; nya destinationsfält eller klientberäkningar behövs inte. Keryx-hjälpens
felaktiga generella "departure hex" är rättad; Codex marching förklarar hemstad
för expedition och avfärdshex för vanlig marsch. Webben visar samma befintliga
Recall-kontroll och serverstatus och verifieras fysiskt. Ingen notifieringstyp
eller tolkning av gammal audit ändras. Vault-plan och verblista uppdaterade.

R ändrar inte skeppsregeln om order vid havet, returpatrull/damaged_return,
eller den äldre returhanterarens beteende om ägarförlust sker **efter** att recall
redan avslutat expeditionsraden. Hemstadens ägande/nåbarhet prövas vid leverans.

## Recept

Bygg `OUT/temenos` med `go -C server build -ldflags '-X main.buildCommit=HASH'
-o OUT/temenos ./cmd/server`. Kör `python3 tools/war_cards_live.py OUT HASH
recall-home`. Verktyget skapar egna PG16/Redis/värld/spelare och tar endast bort
sina egna resurser; SQL är read-only audit. Binärer/cache under HOME, inga
befintliga spelvärldar rörs. `restored` väljer vanlig marsch via read-only
march-preview; 422-kandidater hoppas över och kadensen är 12 s/tick så bilderna
hinner tas medan truppen fortfarande kan nås. Expedition använder 6 s/tick.
Avvisade prov av vanlig marsch sparas i loggarna `live-plain.log` (422-preview)
och `live-plain2.log` (ordern hann inte fångas före ankomst); de räknas inte som
acceptans. Speldagar och spelets orderregler är oförändrade.

Vanlig marsch: tredje färska arm `live-plain` grön med faktisk webborder,
Runner202 → MarchRecalled → garrison i samma grundade stad; inga browserfel
eller OrderDeliveryFailed. Samma rena c7e8496c-binär som expeditionernas armar.

## Kunskap vid återkallelse — uppmätt efter Claudes fråga

**Val:** behåll den redan bestående kartkunskapen och den befintliga regeln att
recall avslutar expeditionen utan expeditionssammanfattning. Rapporten är inte
transporten som låser upp kunskap vid hemkomst i denna implementation. Den
sammanfattar kartan i ett event och en personlig arkivnotis. Att låta just recall
skapa en ny sorts rapport är därför ingen nödvändig del av destinationsfixen.

Spelaren förlorar **sammanfattningen** (speldagar ute, längst bort, antal sedda
hexar, samlad fyndlista), inklusive dess arkivrad. Spelaren förlorar **inte** de
sedda hexarnas terräng/fynd eller deras status som kända. Den separata listan
över vad just expeditionen såg kaskadrensas när missionen avbryts; spelarens
bestående kunskapslager berörs inte.

Spårning, fil:rader i R:

- `server/internal/combat/expedition.go:211-238`: varje avslutat ben sveper den
  faktiska vägen med SweepLiveRadiusAlong och fyller även missionens synlista.
- `server/internal/province/eyes.go:392-400`: svepet skriver bestående
  `player_scouted_tiles`, oberoende av om spelaren läst kartan.
- `server/api/handlers/world.go:1044-1046`: loadRememberedTiles läser det lagret;
  `:371-376` gör hexen remembered/visible, `:377-385` maskerar bara fog. Kända
  fynd förblir alltså tillgängliga utan en hemkomstrapport.
- `server/internal/combat/expedition.go:347-417`: rapporten läser synlistan och
  kart-/stadsrader för att skapa sammanfattningen. `:419-437` skriver event och
  arkivnotis och rensar missionen; ingen kunskapsprojektion skrivs här.
- `server/internal/combat/recall_redirect.go:215`: recall rensar missionen.
  `server/db/migrations/159_unit_expeditions.up.sql:27` kaskadrensar bara dess
  `unit_expedition_seen`, inte spelarens `player_scouted_tiles`.

`knowledge.log`: färsk PG16/migration160; riktig StartMarch → första arrival →
recall → hemkomst. **39 bestående kända hexar**, missionens synlista **39→0**,
**0 förlorade hexar** ur den exakta före-mängden efter både leverans och hemkomst,
**koppar (1,2) fortfarande känd**, **0 ExpeditionReport-notiser**. Ny regression
`TestRecallExpeditionPreservesMapKnowledge` är grön. Produktionskod, tidigare
fullsvit/vet/JS/mutationer och fysiska bevis är oförändrade (`c7e8496c`).
