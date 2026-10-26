# Klientdelen — bud följer enhet

Status: byggd och riktat verifierad, inte själv integrerad/deployad. Root äger server, API, verklig HTTP/worker/BILD/Keryxacceptans, fullsvit och integration. Klientkontrakt: `following_unit`, `pursuit_legs`, `following_wait` på egna order i `unit.orders_en_route`, `MapMessengers` och dispatchsvar. Frånvarande flaggor behåller äldre presentation.

## Resultat

Webbens befintliga `runner_status.js` är fortsatt den enda läsaren för War-kortets order-Runner. Den visar nästa möte, ny fysisk rutt eller väntan, aldrig garanterad leverans. Keryx `unit list` behåller namngivet förband och samma servermetadata. När en äldre mötestid passerat säger webben att nästa mötespunkt kontrolleras; väntan saknar mötes-/leverans-ETA. Passage väntar utan falsk mötestid. Dispatchkvitton för march/recall/redirect/stance använder motsvarande kvalificering. Gamla flagglösa kvitton och pendingrader behåller tidigare text.

Codex `marching.md` och `runners.md` beskriver att en ny landorder fortsätter efter missat möte, att nästa mötestid kan ändras, och att ordern gäller först när Runner och förband möts. Inga nya notifieringstyper, offentliga informationsfält, CSS eller grafik införs här.

## Rött → grönt

- `client-js-baseline-red.log`: nästa möte/ny rutt/väntan saknades i de faktiska befintliga rendererfunktionerna; riktiga assertionsfel.
- `client-keryx-baseline-red.log`: JSON med nya metadata tolkades men tappade flaggorna och lovade ”runner reaches it”; riktiga assertionsfel, ingen compilefail.
- `client-receipt-baseline-red.log` och `client-receipt-keryx-baseline-red.log`: march/recall/redirect/stance-kvitton gav gamla leveranslöften trots ny flagga. Go-provet kör verkliga Cobra-kommandon mot HTTPtestserver.
- `client-wait-baseline-red.log`: väntande ny order saknade väntetext och visade mötestid.
- Slutgrönt:100JS (runnerstatus, War-kort, stance, marchmenu och14Codexprov), hela Keryx-paketet genom `tools/gotest.sh ./cmd/keryx` på egen ny216DB/renenv, web/keryx/codex dayguards. Se `client-js-green.log`, `client-keryx-green.log`, `client-day-guards.log`.

## Täckning och avgränsningar

| Prov | Täcker | Täcker inte |
|---|---|---|
| `TestOrdersEnRoutePursuitUsesNextMeeting` | Verklig JSONdecode till Keryx schema, första/nyttben, ursprungligt ordermål, okänd mötestid och inget leveranslöfte | Riktig server/DBleverans eller alla unitlistflaggor |
| `TestOrdersEnRoutePursuitWaitHasNoDeliveryETA` | Serverns väntflagga visas utan gammal mötestid | Backendens beslut om väntan |
| `TestPursuitDispatchReceiptsUseMeetingEstimate` | Fyra verkliga Cobra-kommandon march/recall/redirect/stance, JSON från HTTPtestserver, ny flagga kvalificerar tid och håller ursprungligt mål | Genuin dispatch/worker/DBfysik; JSONmode är oförändrad råpass-through |
| `TestStancePursuitWaitingDoesNotPromiseMeeting` | Stance väntan både explicit following_wait och awaiting_passage, ingen ETA | Övriga passageövergångar |
| `order pursuit:*` i runner_status.test | Nya/flyttade/passerade/okända möten, passage med escapad port, tillfällig väntan utan tid; äldre tester behåller flagglös text och rescue-sekretess | Faktisk layout och pollning |
| `pursuit pending belongs…` | Riktig War-kortrender med eget orderunitID och namngiven3rdSpearmen; annan enhet får inte raden | Serverns FOWfilter, nativebrowserlayout |
| `recall and redirect receipts…` | Samma receiptfunktion som faktisk recall/redirect använder, flagga kvalificerar tiden | HTTPsändningen från webben |
| `march receipt:*` | Riktig sendMarch-kontrollflöde med simulerat202/uppfräschning, tid kvalificeras i faktisk kvittoyta | RiktigAPI, nätavbrott och alla blandade grupper (äldre prov finns) |
| `stance receipt:*` | Faktisk gemensam stanceformatter får ny flagga och håller gammal fallback | Backendvalidering och stridsutfall |

Inga påståenden om nativeBILD, verkligt Keryx mot Temenos eller fullsvit görs av klientagenten; root ansvarar för det slutliga beviset. ServerWIP låg samtidigt i samma arbetskopia men har inte redigerats/stagats av klientagenten.

## Föreslagna fysiska mutanter per kärnfil

**Inte körda som mutanter här.** Raderna nedan verifierades enbart byte-för-byte mot avsedd substitution i temporära kopior. Kör dem i separat clean checkout, därefter namngivet prov och exakt restore. Root väljer slutmatrisen.

### server/cmd/keryx/cmd_unit.go

```sh
sed -z -E -i 's#json:"following_unit"#json:"follow_unit"#' server/cmd/keryx/cmd_unit.go
```

Förväntat assertion-rött: `TestOrdersEnRoutePursuitUsesNextMeeting`.

### server/cmd/keryx/cmd_unit_orders_en_route.go

```sh
sed -z -E -i 's#if\ o\.FollowingUnit\ \{#if false \&\& o.FollowingUnit {#' server/cmd/keryx/cmd_unit_orders_en_route.go
```

Förväntat assertion-rött: `TestOrdersEnRoutePursuitUsesNextMeeting`.

### web/static/js/megaron/ui/runner_status.js

```sh
sed -z -E -i 's#if\ \(runner\.following_unit\)\ \{#if (false \&\& runner.following_unit) {#' web/static/js/megaron/ui/runner_status.js
```

Förväntat assertion-rött: `order pursuit: next meeting time`.

### web/static/js/megaron/ui/stance.js

```sh
sed -z -E -i 's#if\ \(d\?\.following_unit\)\ return#if (false \&\& d?.following_unit) return#' web/static/js/megaron/ui/stance.js
```

Förväntat assertion-rött: `stance receipt: following-unit time`.

### web/static/js/megaron/ui/marchctx.js

```sh
sed -z -E -i 's#dispatched\ \&\&\ first\.data\.following_unit#false \&\& dispatched \&\& first.data.following_unit#' web/static/js/megaron/ui/marchctx.js
```

Förväntat assertion-rött: `march receipt: a following Runner`.

### web/static/js/megaron/ui/drawers/war.js

```sh
sed -z -E -i 's#if\ \(d\?\.following_unit\)\ return#if (false \&\& d?.following_unit) return#' web/static/js/megaron/ui/drawers/war.js
```

Förväntat assertion-rött: `recall and redirect receipts qualify`.
