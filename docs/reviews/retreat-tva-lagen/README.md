# Reträtt — två lägen

Inventering på **oförändrad 2b83b1f6**, skriven före produktionskod. Gren `codex/retreat-tva-lagen`, eget worktree `/home/tk/wt/codex-retreat-tva-lagen`. Claude granskar och integrerar; hash + STOPP.

## Kontrakt

| Fält | Kontrakt |
| --- | --- |
| Problem | Fasta procenttrösklar och fem UI-val gör reträtt mer komplicerad än beslutad kanon. |
| Spelarsanning | Två lägen: Hold to the last man / Retreat when it goes badly (= by_loyalty, standard). |
| Invariant | API tar bara dessa lägen; gamla lagrade fasta trösklar blir by_loyalty. Stridsmotorns sideRouts ändras inte. |
| Scope | Requestvalidering och lagring, en ny additiv datamigration, Keryx, webb, Codex/verblista, prov/rigg. |
| Non-scope | Stridsbalans, routing, ny per-unit pre-battle-inställning, offer-slicen. |
| Acceptans | Förbjuden retreat_at_loss → begriplig400 + mutation; migration normaliserar legacy och lämnar hold/övriga data; färsk full Go/JS/vet; Firefox1280+390 och normalt användarflöde. |
| Stopvillkor | Oklart hur gamla redan avsända bud ska migreras avstäms med Claude; inga historiska utfalls-event skrivs om. |
| Bevis | Inventering/baslinje först, persistent migrationsprov, HTTP/domänprov, mutation, faktisk webb/CLI/API. |

Kedjegrind: bevisar nåbar och begriplig stridsorder, utan ny brons-/elitmekanik.

## Inventering före kod

| Lagringsyta | Form/ägare | Åtgärd |
| --- | --- | --- |
| player_world_records.retreat_default | JSONB från migration145. {} = loyalty; retreat_at_loss = fast andel KVAR; hold_to_last_man = håll. Skrivare combat.SetRetreatDefault; kopieras till ny battle_participants via seedStandingOrdersSQL. | Additiv ny migration ersätter fasta trösklar med loyalty; nya validerade defaults endast två lägen. |
| battle_participants.standing_orders | JSONB från migration114. Endast här bor per-unit reträtt, aktiv strid. Skrivare combat.SetStandingOrders (garrison direkt, fält via Runner), läsare battle.sideRouts. | Migrera legacytrösklar, bevara andra nycklar; setter ersätter båda modefälten atomiskt. sideRouts/legacyengine-fixturer orörda. |
| scheduled_events.payload.standing_orders | Pending ScheduledOrderDelivery bär StandingOrdersOrder med Go-fältnamn RetreatAtLoss/HoldToLastMan; levererar till samma setter. | Inventerad livscykelrisk: gamla fasta order får inte återinföra trösklar eller fastna efter ny validering. Avstäms före val av migrationsstrategi. |
| messengers.order_payload | Beseglat brev sparar motsvarande order för passage/rekonstruktion och förlustytor; nyckelform verifieras mot dispatchkod. | Samma legacyorder måste följa samma migrerade läge; inga historiska förlust-/utfallsnotiser skrivs om. |
| events.payload | Append-only utfall StandingOrdersChanged och audit; inte stridens aktiva projektion. | Orörd historik, äldre procentfält förklarar det som faktiskt hände. |
| standing_orders-tabellen | Migration140: återkommande varutransport, inte battle retreat. | Orörd — samma namn men annan mekanik. |
| units | Ingen retreat-kolumn; unit-kort får in_battle, inget nytt förhandsval. | Orörd datamodell. |

API: realm GET/PUT retreat-default (handlers/retreat_default.go); unit POST standing-orders (unit.go1834). Per-unit validering måste ske **före dispatch**, inte bara efter att budet kommit fram. Den gamla settern tillät partiella samtidiga modefält; två lägen blir ett uttryckligt val.

Webb: ui/retreat.js delar presets/body/defaultlabel/realm/per-unit. war.js använder samma body; per-unit visas bara in_battle. Keryx retreat-default har tre mode-flaggor, unit retreat-order saknar by-loyalty och accepterar partiell procent+hold. Codex battle.md och stances.md beskriver procentval. Dessa fyra ytor ändras i samma slice.

[DB-baslinje](baseline-db.txt) kommer från isolerad rigg, **inte produktion**: åtta realm-rows {} och inga battle participants. [Process](baseline-provenance.txt) är cdfe6ab/mig172 och har både äldre aktiv värld och arkiverad BILD-värld; auto-provenance väljer den nyaste arkiverade. Den är inte master2b83b1f6-processen. Källbaslinjens färska Go/JS körs separat.

## Grindar

Pågår. Inventeringen ovan skrevs före implementation. Ny migration läggs efter172; redan publicerade migrationer ändras inte. BILD i bunten; ingen Timothys merge-grind enligt uttrycklig order om ändringen är text/knapp.
