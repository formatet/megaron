# Keryx-speltest 2026-10-15

Spelare: en främling med bara `keryx help` och Codex-artiklarna. Ingen produktionskod rörd.

**Härkomst:** `rigg: commit 2b0c058 · migration 181/181 · värld 1312f0dd · tick 39 · TICK_SECONDS=120 · 2026-10-15 23:04`
(riggen byggd ur worktree på master 2b0c0582, `reset` körd, ~90 min väggtid, ~dag 0 → 41).
Fyra Wanaxer registrerades via `acceptance.sh player` (världen startar först vid 4); bara `speltest` spelades.

## Hur långt i kedjan

- **Geografi: delvis.** Grundade kustmetropol Thapsos (48,15), gåvogalär, tre explorer-turer med galären
  (hull 5 → 1 av stormar), spearmen-explore norrut, kolonin Kassiteros (48,11) intill fyra tennhex,
  tennmina köad i kolonin (klar ~dag 43), silvermina, tempel, varv, hamn. **Ingen koppar hittad**, bara
  västlig strand (37–39,17–19) sedd på avstånd. Oraklet (Potnia's Vision) kastat vid kharis 4,7 → "gudarna tiger".
- **Brist: ja** — tenn på egna landmassan; koppar ej hittad.
- **Brons/elit: nej.** Stopp: (1) koppar finns på annan landmassa, galärens upptäcktsfärd (30 dagar) pågick vid avslut;
  (2) oraklet är i praktiken stängt för en ny Wanax (se A1); (3) varje bygge kostar 2 min × dagar och stenen är flaskhals (se C).

## Logg (förkortad)

1. `keryx help` → `join` (redan joinad av skriptet) → `founding status`, `notifications` (öppningsdispatchen), `map`. OK.
2. `founding settle` utan `--yes` på (49,14): prognos "Inland ... starves ... lasts ~2400 days" (B1). Avbröt, marscherade värden till (48,15) → "The world has not begun — 1 of 4 Wanaxes have arrived" (väntat, texten är tydlig och Codex förklarar det).
3. Registrerade 3 till; världen blev active direkt. Grundade `founding settle --yes --name Thapsos`: galär-gåvan kom, `livestock 50.0` ✓ (bekräftat).
4. `place` timber/stone; `build --type temple` → fel (A2/float). `rite` före tempel → "temple required" (först vid kast).
5. Galär `unit march --intent explore`, spearmen `march` + `--intent colonize`, `transfer` till kolonin (14 dagar), `allocate --cult 15`, `unit repair`, `rite`.

## Bekräftat enligt ordern

- `web/static/codex/founding.md` säger nu "city centres stand at least **three** hexes apart ... catchment may share ground" — rättat (2b0c0582). OK.
- Boskap vid grundning: `goods` visar `livestock 50.0`. OK.

## A — går bet / yta saknas

**A1. Kharis-spiralen gör oraklet (enda koppar-sökning utan skepp) oåtkomligt för en ny Wanax.**
Vid grundning `Kharis 25 (Suspicious) · cap 25`; `ticklog` dag 16–20 visar `KharisMissedMaintenance` varje dag; vid dag 20
`Kharis 1 (Wrathful)`. Först 16 dagar efter bygget är templet klart; med `allocate --cult 15` blir det `net +0.3 per day`,
så kharis 5 nås först ~14 dagar senare (dag ~39). Kastet då: `rite --prayer minoan_potnia_vision_v2 --yes` →
`Failed: The gods are silent. Your offering was received, but they do not answer.` / `Kharis spent: 4`.
Codex `rites.md`: "Its chance is essentially your standing" — standing 5 av cap 50 ≈ 10 %. Öppningsdispatchen och
`bronze.md` lovar oraklet som väg ("you can ask the oracle"), men en spelare som följer dem bränner 4 av sina 5 kharis på ett kast med ~10 %.
Yta: temenos (balans) + codex (löftet). Förväntade: antingen att kharis inte rasar till golvet före första templet, eller att Codex säger "räkna med ~40 dagar".

**A2. Bygge nekas på exakt kostnad p.g.a. flyttalsrest.** `build --type temple` vid 24 sten:
`Error: insufficient_goods: stone (need 24, have 24.00, 0.00 short) (HTTP 422)`. DB: `23.999999999999996`.
Reproducerades en gång till (`mine`, `need 16, have 16.00, 0.00 short`). Den som siktar på en exakt summa får ett självmotsägande fel och
måste gissa att vänta en dag. Yta: temenos (`economy` jämför flyttal; avrunda eller epsilon).

*(A2 blockerar inte, men är det som en främling först tror är en bugg i spelet. A1 blockerar kedjan.)*

## B — obegriplig eller felaktig text

- **B1. Grundningsprognosen motsäger sig.** `founding settle` på (49,14): `Starting stock 1200 grain lasts ~2400 days — the city starves.` (2400 dagar = svälter inte).
  På (48,15): `production ~0 per day − consumption ~0 per day = net +0.0 per day` — konsumtion för 1000 människor visas som ~0; verkligheten efter grundning: `consum 5.0`, `prod 0.5`.
- **B2. Kolonins prognos ljuger / motsäger.** `Starting stock 0 grain — the city is self-sufficient.` + `ColonyFounded: Kassiteros is self-sufficient` men första dagen: loyalty-händelse `-1 starving (grain stock below the comfortable-buffer reference)`, `Loyalty: 2`, `Grain 2 ... coverage 3.0 days`.
- **B3. Föråldrade byggtal i varning.** `keryx status` i kolonin: `Timber production ~0 — timber gates harbour (140)/barracks (80)/foundry (80)/temple (60)`; `build --list` säger harbour `timber×16`, barracks `timber×8`, foundry `timber×8`, temple `timber×8`.
- **B4. Kharis visas per bosättning.** Kolonin skriver `Kharis 1 (Wrathful)` medan huvudstaden hade 4–5; Codex: realm-pool. Förvirrande vilken som gäller.
- **B5. Avrundad kharis döljer tröskeln.** `rite --list`: `The gods are Wrathful (kharis 5)` → `rite` svarar `requires 5 kharis (you have 4.7, 0.3 short)`.
- **B6. Svenska i engelsk utskrift.** `recruit --list`: `0.6 grain, 2.0 silver (0.6 netto i garnison)` samt dubblerad mening `current stock 409.15 covers ~57 days at that rate — current stock 424.65 covers ~708 days at that rate`.
- **B7. `rites.md` listar fel böner.** Under "The prayers": oracle / harvest blessing / battle frenzy, medan `rite --list` har Potnia's Vision, Walls, Green Shoots, Thread, Passage (sektionen "The Minoan prayers" längre ned stämmer).
- **B8. Expeditionsrapporten säger inget nytt.** `ExpeditionReport: ... saw 173 hexes around (41, 19): silver at (49, 13)` — silvret vid hemstaden, inte det upptäckta (västlig strand, plains). `ExpeditionTurnedHome` säger "nothing there is left unseen" efter 1 dag.
- **B9. `MetropolisFounded` har tom brödtext** i `notifications`.
- **B10. Egen överföring kallas `TradeDelivery`** ("Trade delivered: 20 grain") — det var interna transfer till egen koloni.
- **B11. Tidsuppgifter.** Öppningsdispatch: "olive woodland in 1.5" (dagar), `unit march --preview`/ETA rundar till 2. Caravan 4 hex (Thapsos→Kassiteros, över ett bergsmassiv) = **14 dagar**; mina/tempel "in 3 days (≈22:32)" visar ETA utan att säga att byggtiden räknas från köplats.

## C — småsaker

- `--config` finns inte (ordern säger det); keryx styrs av env `POLEIA_CONFIG` (`keryx help` nämner det inte, bara Codex `keryx.md` pratar `POLEIA_SERVER`).
- Fel från `keryx` skriver hela usage-blocket varje gång (långt, dränker feltexten).
- `unit march`-felet vid marscherande enhet hänvisar till "right-click the map" (webbfras i CLI); `redirect` kan inte bära `--intent colonize`, så kolonistmarsch måste ske i två steg (marschera, vänta, `march --intent colonize`).
- `city` tar positionsargument (`city Kassiteros`) men `status/place/build/map` tar `--province` (som vill ha provins-id, inte settlement-id i `cities` — `settlements` visar båda men `cities` bara settlement-id).
- Kolonin får samma ★ som huvudstaden i `cities`.
- `map --province` skriver "colonize adjacent hex" vid tenn bredvid en redan grundad koloni.
- `unit march --preview` skriver `Sold: 0.45 → 1.5 silver per day in the field` — ordet "Sold" förklaras aldrig.
- Galären seglar vidare med hull 3 → 1 in i stormar; ingen varning/avbryt (ordern kan inte ändras till sjöss, Codex säger det) — första explore kostade 2 hull direkt.
- `rite --list` visar `Ready: no` utan orsak (tempel saknas); felet kommer först vid kast.
- Stenen är flaskhals (hex #12 FULL med 2, övriga hexar 0.5/dag); fyra av tio gubbar på fisk. Första temple+mina+varv tog ~35 min speltid. Inte fel, men "A new city starts with no timber and no stone" är det som sätter takten.
- Codex läst: `getting-in`, `founding`, `welcome`, `city`, `citizens`, `buildings`, `bronze`, `sea`, `colonies`, `trade`, `rites`, `temples`, `kharis`, `keryx`. Behövde **inte** läsa serverkod.

## Fungerade som väntat (data)

Join/status/map/founding-prognos, världsgrind ("has not begun"), placering, hex-bundna byggen (`--hex`), coloni via marsch, `transfer`, `unit repair`, `allocate`, `ticklog`, `roster`, `cargo`, `notifications` efter frånvaro (läsbart), Codex-bekräftelsen på 3-hexregeln och 50 boskap.
