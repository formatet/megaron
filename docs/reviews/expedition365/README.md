# Expedition ceiling 365 ticks

Contract: proves geography. Timothy requests only an increase from 30 to 365 ticks.
The server owns the range; web loads its limits and Keryx forwards --ticks for server validation. Minimum 4, default 10,
area radius, turn/home budget, physical recall and all upkeep rules remain unchanged.
Scope: maximum constant, existing boundary/HTTP tests, Codex sight article.
Acceptance: 31 and 365 accepted, 366 rejected; API exposes 365; clients use server rules.
No visual layout change or new mechanic. Future departure payroll/provisions is vault-only.

Baseline on 31460677: targeted expedition/recall/HTTP preview tests GREEN on a fresh migration-193 DB.
Changing the existing lifecycle scenario to 365 and HTTP cases to 31/365/366,
with the old production ceiling still 30, made both named tests RED:
`TestExpedition_AreaKnownTurnsEarlyAndReportsFinds` (365 rejected) and
`TestMarchPreview_ReadOnlyAndTiming` (31 rejected). This is the physical old-limit
mutation/control; restoring 30 would reintroduce these failures.

Validation: full `tools/gotest.sh` GREEN on fresh migration-193 DB; Go vet combat/handlers/Keryx GREEN;
all 680 JS tests GREEN. Existing HTTP test also checks returned expedition rules,
read-only previews and hidden terrain. The lifecycle scenario dispatches a real
365-tick mission through StartMarch, runs arrival handlers to garrison and checks
its early return and discovery report. Existing physical recall tests remain in suite.

Four surfaces: API exports ExpeditionMaxTicks; web configureExpedition uses max_ticks;
Keryx validates duration on the server (no duplicated ceiling); Codex sight now says 365.
Only a tunable and documentation change: no layout, schema, payroll, provisioning,
recall or tick-speed changes. No merge/push/deploy by this branch (AGENTS.md roles).

