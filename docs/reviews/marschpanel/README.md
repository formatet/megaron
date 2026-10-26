# Marschpanelen: välj enheter vid namn

Kontrakt före kod2026-10-26.
Problem: standardantalet omfattar hela första gruppen och val av en annan namngiven kohort är omöjligt; förhandsvisningsfel hindrar inte sändning av för kort expedition.
Spelarsanning: se och välj exakt namngivna enheter före sändning; en för kort expedition lämnar inget bud.
Invariant: enbart valda ID får order; blandad march/redirect behåller sin fysiska Runner-väg. Förhandsvisning får inte avslöja dold rutt/restid. Domänens befintliga tur/returregel ändras inte.
Scope: webbens enhetsval+förhandsvisning+sändning, befintlig serverbudget och dess prov, Keryx explicit ID/preview, Codex marching.
Avgränsning: bud som följer en flyttad enhet är nästa slice; inget annat budbeteende eller balans ändras.
Acceptans: högst en förvald enhet; val av annan namngiven enhet; samma IDs preview/send; för kort budget stoppar skickandet; namn syns före sändning på fyra ytor.
Stopvillkor: ny informations-/mekanikkonflikt tas direkt till Timothy.
Bevis: baseline-röda prov, verkliga API-budgetprov, selektions- och asyncprov, mutanter, färsk fullGo/JS, nativeFirefox1280/390 och livehash/grundtillstånd.
Gate: proves läsbar ordergivning i kedjans geografisteg.

## Resultat

Namngivna checkboxar väljer exakt enhet; högst en förvald. Preview och send delar urval. Både webb och Keryx kräver lyckad read-only expeditionsvalidering före första order-POST. En giltig men okänd ETA är tillåten. Stängd/ändrad panel invaliderar väntande sändning; ett nytt val kan skickas efteråt. Keryx tar exakt visningsnamn eller IDprefix och nekar tvetydiga namn. Codex beskriver samma flöde.

Serverns befintliga budgetmekanik är oförändrad. Budgetavslag får en additiv intern kod så att preview maskerar dold minimitid; gammal POST-/eventtext behåller sin betydelse. Inga nya migrationsfiler; migration215.

Native Firefox hittade att feltexten blev för smal och att avbruten forecast lämnade ”Estimating arrival…”. Feltext får egen rad och send tömmer avbruten forecast; nytt test assertion-rött före fix. Ny capture ersätter första försöket.

## Grindar och bevis

- Kod: hel färsk Go på764b0bc5 (oförändrad slut-Go), alla26 paket gröna, world147.291s; slut-JS794/794, vet och fyra dayguards gröna. Go via tools/gotest.sh på egen tom215DB och ren miljö.
- Mutation: [täckning, exakta sed-rader och avgränsningar](coverage-and-mutations.md). Separat checkout, assertion-rött och exakt restore; explorativ överlevare redovisad.
- Visuell/användare: [native Firefox1280/390](bild/index.html), verkliga order-POST/DB, endast vald3rdkohort, förkort expedition nollPOST/nollbud. Scenfixturer redovisas; ingen delad/livevärld används.
- Drift: pending integration. Läsbaslinje [ground-before.json](ground-before.json); efter deploy verifierar tools/marschpanel_deploy_verify_20261026.py processhash, dirty=false, samma värld/grundtillstånd och ändrade statiska bytes.

## Kända avgränsningar

Bud som följer flyttad enhet är nästa slice. Ingen mekanik/balans eller persistent eventtolkning ändrad. Nya klientprov använder HTTPtestserver/DOMstubbar; riktiga layouten/orderflödet bevisas separat med Firefox. Alla terräng-/skeppskombinationer och isolerad outboundbudgetgrind täcks inte av nya prov; se matrisen.
