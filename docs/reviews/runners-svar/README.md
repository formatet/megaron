# A4b — Runners: gruppen *Waiting for an answer*

Gren `claude/runners-svar` (från master 2aed60f9). **BILD-grind: Timothy ser B1 (800 px) och B2 (390 px) före merge.**

## Kontrakt

- **Problem.** Ett eget bud som nått mottagaren försvann ur Runners-fliken (statusen `delivered` lästes inte) — det stod i
  mottagarens stad och väntade, men spelaren såg det bara som ett brev i Correspondence, utan att veta när budet ger upp.
- **Spelarsanning.** *Diplomacy → Runners* och `keryx runners` har gruppen **Waiting for an answer**: till vem och vilken stad,
  sedan när budet står där, och när det går hem om ingen svarar ("heads home … if unanswered").
- **Invariant.** "Sedan" = `messengers.arrives_at` (samma källa som inkorgens `arrived_at`). "Går hem" = den väntande
  `MessengerStayEnd`-händelsens tick (obearbetad, ej död), omräknad med världens ankare — samma tidpunkt som
  `StayEndHandler` vänder budet. Ingen ny regel, inga knappar (svaret är mottagarens). Främmande vy oförändrad.
- **Non-scope.** Ingen *Call back* från mottagarens hov (ingen sådan regel finns). Offert-specifik text.

## Bevis

- `TestRunners_AwaitingAnswer` (färsk DB mig197): grön. Mutationer: (1) ta bort `processed_at IS NULL` → röd
  (leaves_at läser fel tick); (2) ta bort `'delivered'` ur statusfiltret → röd (gruppen tom). Återställt → grön.
- `TestRunners_GroupsAndIntegrity` grön oförändrad. keryx `TestRenderRunners` grön. `diplomacy_runners.test.mjs` 3/3.
- BILD: `B1.png`, `B2.png` — riktiga drawern i Firefox via `tools/runners_flik_shot.py docs/reviews/runners-svar`.
