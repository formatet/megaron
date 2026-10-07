# H — gemensam marschmeny

Bas: `codex/recall-all-enkel` @ba96c79b (Claudes styrning 21:59). Bevisar geografikedjans nåbara orderyta. BILD väntar Timothy i H–K-bunten; ingen merge/push/deploy.

| Kontrakt | Innehåll |
|---|---|
| Problem | Kartmenyn börjar med inga enheter; War kräver separat Q/R-inmatning. |
| Spelarsanning | Första valda gruppens lediga enheter är förvalda; War → March väljer en enhet och ett kartmål i samma meny. |
| Invariant | Befintliga per-enhetsrutter, FOW, redirect, explore, kolonisering och lastat skepps landstigning bevaras. |
| Scope | marchctx.js, war.js, main.js, state.js, number_words.js (gemensamt spelarprosaformat), kartans målklick, map.html; funktionstester och egen isolerad webbrigg. |
| Non-scope | Server, CLI, kanon, War-kortens övriga handlingar (I), nya färger/CSS-klasser. |
| Acceptans | Alla lediga i första gruppen förvalda; övriga grupper noll. More gömmer längd/namn/stance. War filtrerar exakt vald enhet. Landstigning/kolonilast har samma body. Mobil har normalt målklick. |
| Stopvillkor | Om förmågan kräver regeländring, fråga Claude/Timothy; bygg övrig UI oberoende. |
| Bevis | Oförändrad bas actual health/mig160, desktop/390px före/efter, JS kontraktsprov, fysisk mutation, färsk spelarresa med riktiga POST och read-only audit. |

## WIP checkpoint — paus 2026-10-07 22:16

**H är inte slutöverlämnad.** Claude vidarebefordrade Timothys paus: spara rent checkpoint, starta inget nytt. H–K baseras på recall-all-enkel enligt Claudes senaste order. I–K har inte startats.

Klart: kod 981d2411 +8eef7147 delar War/map-menyn, förväljer första gruppen och visar extra val under More. War låser vald enhet och accepterar vanligt kartklick (mobil) eller högerklick. Skeppslandning är flyttad till samma per-enhets-POST/body; kolonisering av last har cargo_intent/name. Server/CLI oförändrade från basgrenen. number_words.js är ny gemensam prosaformatterare för följande UI-slicer (inputvärden är siffror).

Bevis: baseline actual health **11246fb1/migration160**, gamla webbassets från **ba96c79b** via git archive; två manuellt valda enheter → två UnitMarchOrdered-audits → båda garnison. Separata färska slutriggar actual health **8eef7147/migration160**: map förvalt två → två order/audits/garnison; War normalt klick/tap → exakt vald enhet/order/audit/garnison. Desktop/390px före/efter finns i live-*. Inga browserfel, SQL-mutationer eller meny-overflow. **389 JS gröna**, full tools/gotest.sh fresh PG16/mig160 grönt (world86.160s), full env-i vet grönt. Go kördes vid981d2411; efteråt endast UI-prosa/JS/provverktyg ändrat. Tre fysiska mutationer (quantity/pin/cargo_intent) → namngivet assertionrött → restored grönt.

**Återstår före H-handover:**
- Landstigningsriggen visar vald lastad galley korrekt och når befintlig POST, men vårt mål inom stadens upptagningsområde är ägt: server422 "is not open, unclaimed land". Ingen grön landing-resa hävdas. Välj verkligt känt, öppet, OÄGT kustmål genom normal scouting/API; verifiera lastens positioned och skeppets hemkomst. JS-kontrakt täcker body och cargo-colonize/name, men hela kolonilandningsresan är ännu inte livebevisad. Se live-land/failure-ui.txt +server.log (aktuell run, äldre failed-arm-bilder räknas inte som grönt bevis).
- Rätta Codex marching.md: War → March väljer enhet, sedan kartmål; Stance/expeditionslängd/namn under More. Ändrad menylabel är "on the march → redirect by messenger". Artikel säger ännu gamla labeln. Ingen ny spelregel.
- Bedöm om statisk kolonilastförhandsvisning behövs: generisk landkoloni-forecast döljs vid landstigning (gammal Warpannel hade ingen sådan). Bevara purse-kvitto från servern. Kontrollera att ordval/numrerad medlemslista är rätt avgränsning av orderns "tal i ord".
- Lägg verktygets reproduktionsrecept i tools/README.md, komprimera slutrapport, uppdatera BILD/todo/board och hand över till Claude. BILD väntar Timothy i bunt. Inget merge/push/deploy.

Rigglärdomar: landmål filtreras på faktiska terrainvärden (mountain_limestone/red, coastal_sea/deep_sea); mål på hav kunde annars testa skepp i stället för landgruppen. Stäng gammal Inspect före kartklick och vänta tills drawer-övergången är klar via elementFromPoint. Expeditionslängden i provet är fjorton, inom serverns intervall; minsta värdet kunde inte nå och återvända. Auditen heter UnitMarchOrdered, inte MarchStarted. Kasserade armar är inte regressioner eller bevis.

## Resume — exakta kommandon

Läs delad `.agents/chat.log`, `CLAUDE.md`, relevanta vaultdokument och `.agents/order-forenkling.md`; kör status/worktree-list och återclaim innan kod. H-trädet är `/tmp/megaron-codex-march-simple-20261007`, huvudträdet stannar på master. Alla egna riggprocesser och containers stängda vid checkpoint; andra agenters riggar ej rörda.

```sh
git -C /tmp/megaron-codex-march-simple-20261007 status --short
git -C /home/tk/Projects/megaron worktree list
# Kör följande från H-trädet:
node --test 'web/static/js/megaron/**/*.test.mjs'
python3 tools/simple_march_mutations.py /tmp/megaron-march-simple-mutations
# Efter nödvändiga rättningar/commit, använd verklig commithash i bygg och rigg.
# Från H-trädets server/:
env -i HOME=/home/tk PATH="$PATH" go build -ldflags '-X main.buildCommit=8eef7147' -o /tmp/megaron-march-simple-map/temenos ./cmd/server
env -i HOME=/home/tk PATH="$PATH" go build -o /tmp/megaron-march-simple-map/keryx ./cmd/keryx
# Från H-trädets rot, egna färska DB/Redis varje anrop:
python3 tools/simple_march_live.py /tmp/megaron-march-simple-map 8eef7147 map
# war och land kräver samma två nybyggda binärer i respektive OUT-katalog.
python3 tools/simple_march_live.py /tmp/megaron-march-simple-war 8eef7147 war
python3 tools/simple_march_live.py /tmp/megaron-march-simple-land 8eef7147 land
# Land ovan reproducerar återstående provmålfelet; rätta först målval/scouting.
# Baslinjeassets, reproducera oförändrad ba96-webb utan att röra parkerade träd:
mkdir -p /tmp/megaron-march-assets-before
git -C /tmp/megaron-codex-march-simple-20261007 archive ba96c79b web | tar -x -C /tmp/megaron-march-assets-before
python3 tools/simple_march_live.py /tmp/megaron-march-simple-baseline 11246fb1 baseline /tmp/megaron-march-assets-before/web
# Baslinje-OUT ska innehålla binär med buildCommit11246fb1; backend samma ba96.
```

Nästa steg: slutför H ovan, därefter I (War-kort + NaN-vakt), J (Production), K (text/inline). Varje egen gren ska ha bas enligt senaste Claude-styrning; återläs chatten före grenval. number_words.js bör ha EN gemensam ägare även om slicer ligger parallellt på recall-bas. Claude integrerar och avgör stackning.
