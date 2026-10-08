# L Inspect — BILD

## Sammanfattning

Främmande stads Culture/Walls-block/DP ersätts med **Owner + Defence: Defended / No defenders seen / unknown**. Host visar **Food lasts** och **Escort pay lasts** i game days/tal i ord. Samma dataåtkomst, knappar och FOW; sena armésvar får inte ändra en ny panel. **Ren klientfix; ingen egen merge/push/deploy.** Bas master887b70b2, fungerande regression före fix **e9be528f**, slutlig klientkod **c3482a9b9489f7e65dc30850fd36550a7868052d** (faa121d2:s styrkeord ersatta enligt Claudes precisering03:00).

| Grind | Bevis |
|---|---|
| Baslinje | **422 JS grön** på oförändrad master; tre regressioner named röda före implementation. Actualhealth **887b70b2/mig160/ok**, fresh30×30: vanlig register/join/Host, två march202 på sammanlagt9speldagar till riktig Kyme genom FOW, **Culture/Walls/200DP**. SendMessenger201, March here rätt känd stadsdest och verklig fogpanel. [API/DB-läsmetadata](baseline/proof.json), [rött](logs/regression-red.log), [stad390](baseline/foreign-city-mobile.png). |
| Kod | **425 JS grön**, fysisk /24 · felaktig försvararfrånvaro · stalesvar i fog · Culture-block → fyra **named röd/restored grön**. Full tools/gotest.sh **ny PG16/mig160 alla paket**, world90.068s; ren env-i **go vet ./... exit0**. [Loggar](logs/). Server/keryx/CSS-diff tom; Horde/Screen-Codex uppdaterade. |
| Semantik | Defended om de tidigare visade murarna eller någon av de tre tidigare DP-typerna finns; No defenders seen om både walls och armé är känt noll; rowlabel Defence + unknown vid saknat underlag. Ingen combat-styrkevärdering/tröskel. Grain är Food; silver är Escort pay. ticks_left betyder redan game days: ingen /24 eller wall-clock. null behåller indefinitely. |
| Användare/BILD | Riktiga fresh56×40-armar med **actualhealth c3482a9b9489f7e65dc30850fd36550a7868052d/mig160/ok**. Host vandrar via vanliga march-preview/march202 tills Kyme faktiskt syns genom FOW; verklig army200→Defended. **Send Messenger201**, March here→rätt dest/known, riktig fog utan ägare/defence/message-knapp; desktop1280×900 +390×844, ingen panel-overflow eller browserfel. [Efter API](after/proof.json), [Host390](after/host-mobile.png), [stad390](after/foreign-city-mobile.png). [Ren repetition API](repeat/proof.json): fem march202 på sammanlagt26speldagar, samma kontroller gröna. [Host desktop](repeat/host-context-desktop.png), [Host390](repeat/host-context-mobile.png), [stad desktop](repeat/foreign-city-context-desktop.png), [stad390](repeat/foreign-city-context-mobile.png), [fog390](repeat/fog-mobile.png). Första efterarmen: fyra march202 på23speldagar. |
| Provenance/drift | Fulla c348-binären SHA256 **d03a49521e8ff0f17228fceae4635226a155c74c919027899c84805ff5e8c8bf**; efter/repeat använder samma binär (symlink). Baseline-UI arkiverad887b; oförändrad musik symlinkas för att undvika arkivdubbletter. Go/vet på faa121d2; inga Go-filer ändrade genom slicen. BILD lämnas till Claude enligt nattens delegation; ingen deploy. |

## Slice-kontrakt

Problem: främmande stad visar Culture/Walls-block/DP; Host förråd blandar tick och verklig tid.
Spelarsanning: owner + Defence i ord; Host visar food/pay i game days och tal i ord.
Invariant: samma dataåtkomst/FOW och alla kontroller; inga nya mekaniska försvarströsklar, ingen silver→mat-förväxling.
Scope: map.html/map.js paneltext, Codex och prov/rigg/BILD.
Non-scope: server/keryx/CSS, fow/renderer/ordrar, andra förenklingsslicer.
Acceptans: Culture/Walls/DP bort; Defended om redan visade murar eller positiva redan visade markförsvarare, No defenders seen om båda känt noll, Defence unknown när underlag saknas; owner/allied bevarat; controls/FOW bevarade; finite/infinite förråd från ticks_left utan skalning.
Stopvillkor: kanon/serverändring krävs → Claude. Hash och stopp efter EN slice.
Bevisplan:422JS renbaslinje, actual map-klick regression först och fysiska mutationer; riktig fresh PG16/Redis register/join/Host+kontaktstad via vanliga API:er, desktop390 före/efter BILD; full fresh Go/vet. Bevisar geografi→brist-grindens panelnåbarhet.

## Experiment och förkastat

| Ändring/hypotes | Utfall/beslut |
|---|---|
| Färre rader + ord från samma data | Culture/Walls/DP bort, owner/allied/Send Messenger/March here/enhetsknappar oförändrade. Defended/No defenders seen följer Claude03:00; strong/weak är förkastat som styrkeomdöme. |
| Sent armésvar | SelectedHex + radens synlighet kontrolleras före uppdatering. Samma förfrågan som förut; fog fetchas aldrig. Actual-handler regression och mutation provar ett fördröjt svar efter byte till fog. |
| Stubben före kod | Första regressionscommit3cf4681d saknade querySelectorAll i DOM-stubben. **Det var inget spelbevis**; stub/template-path rättade i e9be528f medan produktionen var oförändrad. Endast e9be:s named beteenderött används. |
| Arkiv/full Go mot diskquota | Första arkiv/build/full-Go förkastade. Egna avslutade proof-binärdubbletter städade (8 J/gray/I,245.8MB med [hash-audit](logs/own-proof-binary-cleanup.json)); gamla baslinjebinärer/musikdubbletter också rensade. Ny full fresh Go grön. [Avvisad Go](logs/go-quota-rejected.log). |
| För liten värld / fel ö |16×12 nekas korrekt av serverns minimum30. Tidiga spelare hamnade på olika öar; styrkeords-arm och osäker/avbruten vandring förkastade. [Loggar](rejected/). Finalrigg väljer ett **par vanliga spawns** på samma landmassa via READ ONLY metadata; ingen DB-fixtur/teleport eller klientdata injiceras. |

## Recept

```sh
node --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 tools/inspect_mutation.py /tmp/inspect-mutations
tools/gotest.sh
# Från server/, ren env-i HOME/PATH: go vet ./... och go build
# go build -ldflags '-X main.buildCommit=<full hash>' -o OUT/temenos ./cmd/server
python3 tools/inspect_live.py OUT '<build hash>' after
# Repeat: annan OUT/ny DB med symlink till exakt samma fulla temenos.
# Baseline887b byggs separat; git archive887b web (musik kan symlinkas):
python3 tools/inspect_live.py BASELINE_OUT 887b70b2 baseline ARCHIVED_WEB
```

Riggen skapar/städar egna PG16/Redis/processer och har clean env/tick6s. READ ONLY `SELECT landmass_id` väljer aktörer bland vanliga register/join, vars råa host/landmass metadata sparas utan tokens. Ingen speldata skrivs via SQL; alla förflyttningar, grundning och brevet går via vanliga spelar-API:er. Browserns cache stängs med CDP och `/dev/shm` används; karta/unit/markerdata fabriceras inte. Kameran centreras endast för verkliga klick. Ingen march-POST skickas av March here-provet (det öppnar menyn). Baseline30×30; slutligt verktyg/efter/repeat använder56×40, samma panelkontrakt. Olika färska genererade världar jämförs för UI-beteende, inte pixelidentitet.

## Metodisk lärdom

- Välj två naturligt nåbara aktörer för ett UI-prov; olika öar är ett navigationsscenario, inte ett panelprov.
- Named rött ska bero på spelarens felbeteende, inte en trasig DOM-stub.
- Ett ord om observerade försvarare får inte smyga in en bedömning av stridsstyrka.

## Kända avgränsningar

No defenders seen/unknown/walls>0/allied, fraktionell och ändlig Food, samt sena svar provas genom actual map-klick med stubbat HTTP i JS. Livearmen har riktiga tvåhundra försvarare, Food indefinite och ändlig silverpay, och verklig fog; inget live noll-armé-/refusal-prov hävdas. Forecast, unitlist, andra terräng-/rural-/own-city-ytor och FOW/server/CLI är oförändrade. BILD bedöms i bunt av Claude enligt delegation.
