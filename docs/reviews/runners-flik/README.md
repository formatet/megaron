# A4 — Runners-fliken under Diplomacy

Gren `claude/runners-flik` (från master 8693be99). Ej pushad, ej mergad. **BILD-grind: Timothy ska se A1/A2 före merge.**

## Kontrakt

- **Problem.** Spelaren har ingen samlad bild av sina bud. Correspondence visar brev, inte budet; väntande bud, bud ombord och
  ordrars bud ligger spridda (utkorg, dispatch, arméns kort), och det finns ingen väg att se andra Wanaxes bud som faktiskt
  står i hens stad eller åker på hens skepp.
- **Spelarsanning.** *Diplomacy → Runners* (och `keryx runners`) visar varje bud du har ute, grupperat efter var det är:
  väntar på skepp · på väg · ombord · på hemväg (· "no word" för bud utan besked). Ett väntande bud säger var, sedan när och varför
  (serverns egen förklaring) och erbjuder *Arrange passage* / *Call back* bara där servern säger att det går. Egna ordrars bud
  visar ordersammanfattningen. Främmande bud syns bara där du vet att de är: **i dina städer/hamnar** eller **på dina skepp**
  — med avsändarens Wanax-namn och plats, aldrig innehåll.
- **Invariant.** (1) `GET /runners` läser aldrig `message_text`, `reply_text`, `trade_offer` eller `order_payload` för någon
  annans bud (ägarvillkoret sitter i SQL:en, inte i klienten) — och egna ordrar går bara via `messenger.OwnOrderSummary`.
  (2) Främmande bud får varken id, slag (brev/offert/order) eller rutt; skeppsägaren ser bara sitt eget skepps mål
  (`heading_to`) och landfall. (3) Ingen ny spelregel: knapparna återanvänder `POST …/passage` och `…/call-back` oförändrade.
  (4) Tider: klockslag för NÄR ("today 18:34"), inga dagnummer, inga uuid i spelartext.
- **Scope.** Läsyta (temenos) + fliken (webb) + `keryx runners` + Codex-avsnitt + verblista.
- **Non-scope / ej gjort.** Bud med status `delivered` som är DINA (väntar på svar hos mottagaren) visas inte (de syns i
  Correspondence). Främmande bud som bara *passerar* ditt synfält på kartan visas inte (kräver kanonbeslut — se öppna frågor).
  Ingen live-uppdatering via WS (fliken laddas om vid varje öppning). Ingen Umami-händelse (se nedan).
- **Acceptans.** Go-testet nedan grönt och rött mot mutation; JS-testerna gröna; bilderna A1/A2 godkända av Timothy.

## Designval: ny endpoint, inte utbyggd utkorg

`ListSent` är per settlement, max 20 rader, och bär `message_text`/`trade_offer`/`reply_text` (brevets innehåll hör hemma där).
Runners-ytan ska vara *bud-centrerad*, världsvid för spelaren och **bevisligen innehållsfri** för främmande bud — det är renast som
en egen endpoint (`messenger_runners.go`) med egen svarstyp, så ett framtida fält i `ListSent` aldrig kan läcka hit. Förklaringstexten
återanvänder `messenger.stalledExplanation` (nu även exporterad som `StalledExplanation`), skeppsvalen `eligiblePassageShips`,
och reglerna för `can_call_back` är desamma som i `ListSent` (utgående bud i egen hamn). `ListSent` är orörd.

## Ändrade filer

- `server/api/handlers/messenger_runners.go` (ny) — `MessengerHandler.Runners`
- `server/api/handlers/messenger_runners_test.go` (ny)
- `server/cmd/server/main.go` — rutten `GET /worlds/{worldID}/runners`
- `server/internal/messenger/passage.go` — exporterad wrapper `StalledExplanation`
- `server/cmd/keryx/cmd_runners.go`, `cmd_runners_test.go` (nya), `main.go` (registrering)
- `web/static/js/megaron/ui/drawers/diplomacy_runners.js` + `.test.mjs` (nya)
- `web/static/js/megaron/ui/drawers/diplomacy.js` — fliken, och `dipArrangePassage` fick valfri 4:e parameter `blockId` (så Runners och Correspondence inte delar DOM-id)
- `web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs` — "exakt två flikar" → tre
- `web/static/megaron.css` — `.runner-*`
- `web/static/codex/runners.md` — avsnittet "The Runners tab"
- `tools/runners_flik_shot.py` — BILD-riggen
- utanför repot: `/home/tk/.local/share/megaron-luna/luna_command.py` rad 15 (`runners` i `ALLOWED`) och `/home/tk/Dokument/myltavault/megaron_verblista.md` (rad + läsyteraden)

## Endpointens JSON (`GET /api/v1/worlds/{worldID}/runners`)

```json
{
  "own": [
    {"id": "…", "group": "waiting", "kind": "letter", "returning": true, "to_city": "Zakros",
     "port_name": "Phaistos", "waiting_since": "2026-10-21T16:22:00+02:00",
     "explanation": "Your runner is waiting in Phaistos: no ship has sailed from there toward Zakros. Any ship that does — yours or another Wanax’s — takes it aboard by itself. You can also send one of your own ships now.",
     "can_call_back": false, "can_arrange_passage": true,
     "eligible_ships": [{"id": "…", "name": "Dawn Gull", "type": "merchantman", "settlement_id": "…", "settlement_name": "Zakros"}]},
    {"id": "…", "group": "on_the_way", "kind": "order", "order_summary": "Third Spears: March to (14,-3); mission: scout",
     "arrives_at": "2026-10-21T21:58:00+02:00", "can_call_back": false, "can_arrange_passage": false},
    {"id": "…", "group": "aboard", "kind": "letter", "to_city": "Amnissos", "to_wanax": "Kassandra",
     "ship_name": "Sea Swallow", "ship_wanax": "Dunios", "boarded_at": "…", "lands_at": "…", "arrives_at": "…",
     "can_call_back": false, "can_arrange_passage": false}
  ],
  "foreign": [
    {"group": "in_your_city", "sender_wanax": "Dunios", "where": "Zakros", "waiting_for": "a_ship", "since": "…"},
    {"group": "in_your_city", "sender_wanax": "Kassandra", "where": "Phaistos", "waiting_for": "your_answer"},
    {"group": "aboard_your_ship", "sender_wanax": "Kassandra", "ship_name": "Dawn Gull", "heading_to": "Amnissos", "lands_at": "…"}
  ]
}
```
`group` (own) ∈ waiting · on_the_way · aboard · homebound · no_word; `kind` ∈ letter · offer · order. Ordningen är serverns.
Egna bud: `status IN ('outbound','returning')`. Främmande: `awaiting_passage` i *din* hamn, `aboard` ett skepp som *du* äger
(`COALESCE(units.owner_id, carrier_owner_id)`, alltså även efter kapning), eller `delivered` hos *din* stad (ej order).
Bud med `CarrierPassengerRescuedV1` ("no word") döljs för främlingar och visas som `no_word` för ägaren.

## Tester (rött före / grönt efter)

| Test | Rött före | Grönt efter |
|---|---|---|
| `TestRunners_GroupsAndIntegrity` (server/api/handlers) — grupperna, förklaring, call-back, order-sammanfattning, **integritet** (kända ord i `message_text`, `reply_text`, `trade_offer`, `order_payload` finns inte i JSON till skeppsägaren eller stadsägaren; Kassandras bud på hennes eget skepp syns inte för Dunios) | mot master 8693be99: `mh.Runners undefined (type *MessengerHandler has no field or method Runners)` → `FAIL … [build failed]` | `ok formatet/megaron/server/api/handlers` |
| `TestRenderRunners`, `TestRenderRunnersEmpty` (server/cmd/keryx) — grupper, `passage`/`call-back`-rader bara där servern tillåter, främmande bud | `undefined: renderRunners` | ok |
| `diplomacy_runners.test.mjs` (3 test: alla grupper/knappar bara där servern säger/inga uuid/inga dagnummer/escaping; riktiga draweren har fliken och hämtar `/runners`) | modulen fanns inte | 3/3 |
| `diplomacy_simple.test.mjs` (flikar) | — | uppdaterad till tre flikar |

**Mutation (egen, fil kopierad undan och återställd med `cp`):**
1. `pps.owner_id = $2` släppt för väntande bud i främmande klausul → `TestRunners_GroupsAndIntegrity` **röd** ("Kassandra's aboard view wrong": hon ser Dunios bud i hans egen hamn).
2. Ägarvillkoret för "ombord" ersatt med `true` → **röd** ("Dunios' foreign view wrong": han ser Kassandras bud på hennes eget skepp).
Originalet återställt (`git diff` tomt för `messenger_runners.go`).

Fynd under körningen: första fullsviten gick röd för mitt eget test — `players.wanax_name` är unik databasvitt och sviten lämnar rader; testet använder nu ett slumpsuffix.

## Resultat

- Hela Go-sviten (`GOTEST_PORT=57441 tools/gotest.sh ./...`, färsk DB, commit 4c203ddc): 25 paket ok, 0 FAIL.
- JS: `node --test $(find web/static -name '*.test.mjs')` → 699 pass / 0 fail (+ `tools/day_guard.py web|keryx|codex|server` PASS).
- Umami: fliken skickar ingen händelse — befintliga flikar gör det inte heller (`verb_telemetry.js` mäter bara skrivande verb; fliken är en läsning, och knapparna återanvänder de redan mätta verben).

## BILD (Firefox, riktig drawer-kod + `megaron.css`, endast HTTP-svaret är fixtur — `tools/runners_flik_shot.py`)

Bilderna visar drawern (420 px bred på 800-bredd; hela skärmen på 390). Fixturen använder mänskliga namn (Wanax Dunios, Wanax Kassandra; Zakros, Phaistos, Amnissos, Knossos).

- `/home/tk/wt/claude-runners-flik/docs/reviews/runners-flik/A1.png` — viewport 800 px. Väntande bud i främmande hamn (Phaistos) med förklaring + *Arrange passage*; väntande i egen hamn med *Call back*; order på väg; bud ombord på Wanax Dunios' skepp; hemväg; främmande bud i dina städer; främmande bud ombord på ditt skepp.
- `/home/tk/wt/claude-runners-flik/docs/reviews/runners-flik/A2.png` — samma data, viewport 390 px.

Bedömt vid 1:1: läsbart, inga klippta knappar; på 390 radbryts metaraden för "ombord" och "bound for" på två rader (acceptabelt). Knapparna återanvänder `btn-primary`/`btn-danger` (versaler, som i Correspondence).

## Öppna frågor (Timothy)

1. **Kanon:** ska främmande bud som bara *passerar* genom ditt synfält på kartan listas i fliken? Jag har inte gjort det (bara bevisad närvaro: din stad/hamn/ditt skepp).
2. **Skeppsägarens namn på egna bud ombord** (`ship_wanax`): visas nu ("aboard Wanax Dunios' Sea Swallow"). Budet vet vems skepp det sitter på, men säg till om det ska bort.
3. **Leveransbud hos mottagaren** (`delivered`, väntar på ditt svar) listas som "waiting for your answer" i dina städer — dubblerar Correspondence något; vill du ha det?
4. Egna bud med status `delivered` (väntar på svar hos mottagaren) finns inte i fliken; ska de ha en egen grupp ("delivered, awaiting reply")?
5. Fliken laddas om vid öppning men inte live (WS) — vill du ha det som egen slice?
