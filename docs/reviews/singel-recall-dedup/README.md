# Singel recall/redirect — kontrakt före kod

Bas master77f584be; gren codex/singel-recall-dedup, egen worktree.
Aggregat-recall @a92c99b3 är orörd och väntar på BILD hos Claude.

| Fält | Kontrakt |
|---|---|
| Problem | Identisk redan köad UnitArrival gör recall/redirect-transaktionen röd efter committed kurirclaim; order tappas utan audit/notis. |
| Spelarsanning | Kurirens order vänder enheten eller ger namngivet OrderFailed och failure-audit. |
| Invariant | Gamla eventtolkningar och N events/delad messenger⇒bara första vänder är oförändrade. Ingen migration, jitterändring eller balans. |
| Scope | ExecuteRecall:s arrival-enqueue, OrderDelivery recall/redirect-felrapport, DB-regressioner/mutationer och egen klientrigg. |
| Non-scope | Aggregatgrenen, andra verb och fryst separat MarchRecall-handler om den inte använder samma ExecuteRecall-väg. |
| Acceptans | Deterministiskt dedup-rött utan jitter; båda verb återanvänder identisk aktiv arrival i TX; annat exekveringsfel ger name/reason/notis+audit; passage/legacy inventerade; full fresh Go/vet och fysisk spelaringång. |
| Stopvillkor | Ändrad fryst eventtolkning eller canon-/integrationskonflikt tas med Claude före ändring. |
| Bevisplan | Baslinje före produktionskod, klockinjicerade verkliga DB-prov, källmutation, full gotest/vet och färsk snabbtick-rigg med faktisk MarchRecalled-audit +hemkomst. |

## Resume checkpoint
Kontrakt skrivet, master77f584be. Ingen produktionskod ändrad.
Nästa: baseline +deterministiska regressionsprov först, sedan fix/verifiering.
Ingen merge/push/deploy; ingen BILD i denna serverslice.

Baslinje: fresh PG16/mig160 recall/redirect/passage/stale-arrival-proven gröna
på oförändrad produktion91a138a5, baseline.log.
Deterministiska regressionsprov (clock exakt tick1, outbound0→3, q2,
retur1→3 och identisk typed UnitArrival redan köad): båda ExecuteRecall-verb
SQL23505; båda delivery-verb returnerar nil efter committed claim men target
förblir4, ingen audit. Separat injicerat UPDATE-fel ger ingen OrderFailed.
Rålogg red-before.log: sex namngivna assertions röda, inga kompileringsfel.
