# Economy→City och grundning inline — BILD

## Sammanfattning

Economy stängs före City; den sista browserbekräftelsen ersätts med inline-varning och två uttryckliga val. Grundningsknappen spärras medan ordern skickas, fel visas som ren text och tillåter retry. **Överlämnad till Claude för BILD/integration; ingen egen merge/push/deploy.** Bas master **41d107e3**, repro/kontrakt **317de00e**, klientkod **34d26d15d18eb6e498dba7f5b67766851c4648dc**; senare endast prov, dokument och Goods-Codex.

| Grind | Bevis |
|---|---|
| Baslinje | Oförändrad **419 JS grön**, sedan båda regressioner namngivet röda. Actual `/healthz`: **41d107e3, migration160, status ok**. Kartknapp öppnar native dialog; cancel0→confirmPOST201 body{}→reload; City-länk lämnar Economy öppen. [API/DOM](baseline/proof.json), [rött](logs/regression-red.log), [före-overlay390](baseline/city-obscured-mobile.png). |
| Kod | **422 JS grön**, full **tools/gotest.sh ny PG16/mig160 alla paket**, world91.871s; ren env-i **go vet ./... exit0**. [Loggar](logs/). Fyra fysiska mutationer: borttagen Economy-close, återinförd browserdialog, borttagen pending-vakt, borttagen gemensam sent-vakt — alla named röd→restored grön i faktiska klickkedjan. |
| Semantik/användare | Två separata färska PG16/Redis-armar: vanlig register/join, verkligt kartklick, inline cancel **0 POST och Host aktiv**, bekräftelse **1 POST201 body{}**, reload och Host borta; verkligt Economy-radklick öppnar rätt province-City, Economy.open=false, State.activeDrawer=city och mapdim synlig. Inga SQL-fixturer. [Efter](after/proof.json), [ren repetition](repeat/proof.json). |
| BILD | **1280×900 och390×844, 1:1**: varning och båda val nåbara; City synlig utan manuell Economy-stängning. [Host desktop](repeat/host-context-desktop.png), [Host390](repeat/host-context-mobile.png), [City desktop](repeat/city-context-desktop.png), [City390](repeat/city-context-mobile.png). Inga horisontella Economy-overflow. BILD till Claude enligt nattens delegation. |
| Provenance | Båda efterprocesserna rapporterar actualhealth **34d26d15d18eb6e498dba7f5b67766851c4648dc/mig160/ok**; samma hela binär SHA256 **4e6d93ab345d899ff6fae1536cde6a9f44a05bc1f6de46a2a6e26162551b4aa9** (repeat symlink). Bevisen gäller serverlevererade assets och vanliga API:er. |
| Fyra ytor/drift | Server/keryx/CSS-diff **tom**; grundningsregler, endpoint/body och forecast orörda. Codex Founding beskriver båda val, Goods övergången. Abandon behåller K-etiketter/beteende (befintliga test gröna). Deploy inte utförd. |

## Slice-kontrakt


Problem: Economy ligger kvar ovanpå City; grundning blockerar med en browserdialog.
Spelarsanning: stadslänken visar City ensam; grundning kräver ett andra uttryckligt klick bredvid handlingen.
Invariant: ingen grundningsorder vid första klick/avbryt; högst en samtidig POST; samma endpoint, body, regler och reload.
Scope: economy.js, map.js, gemensam inline_result.js, Codex och reproducerbara prov.
Non-scope: andra drawer-vägar, regler/API/server/keryx/CSS och reservkön.
Acceptans: rätt province-ID, Economy stängs före City; varningen om permanent upplöst Host; avbryt skickar inget; dubbelskick spärras; serverfel läsbart inline.
Stopvillkor: server-/kanonändring behövs → Claude. En slice, hash och stopp.
Bevisplan: ren419-JS-baslinje, regression först, fysiska mutationer, full fresh Go/vet, verklig register/join/webbgrundning→Economy→City i separata färska PG16/Redis-armar desktop/390, BILD.
Två symptom i samma slice enligt Claudes uttryckliga order: återstående avbrott i klientens handling→vy-flöde; inga domänändringar. Bevisar nåbarheten i geografi→brist-kedjan.

## Experiment och förkastat

| Hypotes/ändring | Utfall/beslut |
|---|---|
| Stäng Economy före öppning | Regression grön; verklig City aktiv, rätt ID/mapdim. Behåll. |
| Återanvänd K-helper med valbara etiketter | Grundning inline, Abandon-defaults kvar, avbryt0/confirm1/dubbelskick/error/retry grönt. Behåll. |
| Omladdning utan kontrollerad browsercache/kartready | Tidiga baseline/repeat gav modulstart-/hit-test-timeout. **Förkastade**, loggar i [rejected](rejected/). Timeout utan instrumentering räknas inte som diagnos av spelkoden. |
| Utökad request/console-logg | En repeat visade **net::ERR_INSUFFICIENT_RESOURCES** på moduler. Ren omkörning låter Chromium använda `/dev/shm`, behåller CDP Network-session med cacheDisabled och väntar på Host/mapdata+två frames före kartklick. Ren repeat grön utan console/pageerror/resursfel. Inga främmande processer ändrade. |

## Recept

```sh
node --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 tools/drawer_founding_mutation.py /tmp/drawer-founding-mutations
tools/gotest.sh
# Från server/, ren env-i HOME/PATH: go vet ./... och go build
# go build -ldflags '-X main.buildCommit=<full hash>' -o OUT/temenos ./cmd/server
python3 tools/drawer_founding_live.py OUT '<build hash>' after
# Repeat: annan OUT/ny DB; symlink till exakt samma fulla temenos.
# Baseline: bygg41d107e3, git archive41d107e3 web till egen katalog:
python3 tools/drawer_founding_live.py BASELINE_OUT 41d107e3 baseline ARCHIVED_WEB
```

Varje riggarm skapar/städar egna containrar/processer, clean env och vanlig spelares API. Browserns HTTP-cache stängs av via CDP; inga API-svar ersätts. `ERR_ABORTED` vid avsiktlig reload (musik/telemetry/founding-request) ignoreras, men status201, faktisk omladdning, alla övriga nätfel/consolefel och DOM/API-resultat kontrolleras. Första rena efterarmen hade samma verkliga flöde utan utökad console/request-logg; sista repetitionen har den utökade instrumenteringen och fulla viewportbilder.

## Metodisk lärdom

- Vänta på kartans faktiska Host-data och renderad kamera före koordinatklick.
- Modulstart-timeout behöver request/console-bevis; grönt efter en delvis laddad vy räcker inte.

## Kända avgränsningar

Server-/nätfel och retry provas med stubbat HTTP genom kartans faktiska registrerade handler; livearmen bevisar lyckad grundning och cancel, ingen fabricerad serverrefusal. Pixel-determinism mellan olika nygenererade världar hävdas inte; renderer/CSS är oförändrade. Övriga drawer→drawer-vägar ändras inte. Varken CLI-grundning eller Abandon-order körs live i denna klientfix; deras regler är oförändrade.
