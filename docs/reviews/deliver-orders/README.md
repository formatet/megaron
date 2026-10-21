# Delivered course orders

## Summary

A Runner's accepted march used to fail if an earlier Runner had already started
that unit marching. A late recall/redirect was refused before dispatch. Newly
sealed orders now replace a moving land course at delivery, or reach the last
announced destination and apply there only while the unit is present.

## Contract

Proves the geography/command part of the gate. Preserve physical travel,
ownership, dispatch-time FOW, terrain/path validation, expedition home, and old
sealed envelopes. Scope: additive envelope metadata, shared march execution and
arrival-job deduplication, delivery recovery, CLI/web/Codex receipts. No navy
command rules, economy, schema, or live-player orders change.

Acceptance: later delivered march works; late Runner may depart; absent unit
cannot be commanded remotely; replay cannot start another march; old envelopes
retain rejection semantics. Same-position late recall acknowledges without
starting a fictitious march. Stop on any ownership/FOW or old-event conflict.

## Baseline and experiments

Baseline `8057fdd9`, migration193: full fresh-DB Go suite green in expedition365.
Live audit: four accepted couriers to one unit; first started, next three failed
because delivery always called StartMarch. Vault `temenos_orderlopare_plan`
explicitly requires delivered march -> redirect, and Timothy2026-10-20 permits
late delivery at the destination.

New envelope fields opt in; old fields retain interpretation. Course replacement
reads saved route/tick time and verifies unchanged course under the unit lock.
An identical pending arrival job is reused. Late orders carry rendezvous and
return/new target; execution rechecks owner/world and the rendezvous position.
Failed-order text is HTML-escaped, includes destination/delivery context, opens
the unit for review, and says explicitly that muting affects all failed orders.

## Gates and evidence

- Code: full `tools/gotest.sh`, fresh193 GREEN; vet combat/messenger/handlers/CLI
  GREEN; 681 JS GREEN before rebase; integrated master b224c25c: full fresh193 Go
  GREEN again and 682 JS GREEN.
- Regression controls: `tools/delivered_orders_mutations.py OUT` physically
  restores moving-course rejection, late-Runner refusal and raw error HTML; all three
  produced named RED. Files restored in `finally`.
- Semantics: delivered/legacy envelopes; late present/absent/moving unit;
  already-home no-op; repeat delivery; API receipt and stored fallback envelope.
- User: `single_recall_live.py OUT COMMIT cli` registers, joins, founds, sends
  explore orders and real recall/redirect Runners, then checks both actual
  garrisons and exactly one audit per command. Fresh isolated PG/Redis, no SQL
  setup or inherited game environment. CLI and real-browser button flows GREEN on separate fresh worlds;
  both ended with both units garrisoned, one recall/redirect audit each and
  no browser errors. The harness had stale migration160 and Runner-label
  assertions; both now follow current schema/current messenger wording.
- Process provenance: isolated `/healthz` reports commit `deliver-orders-proof`,
  migration193, statusok. This labels the tested WIP binary, not a release hash.
- UI classification: FUNCTION (review action, escaping) and TEXT (receipts),
  no new layout family. Short-hex BILD is a separate, unmerged branch.

## Limits

No retroactive repair of old sealed couriers or existing failed orders. Moving
ships retain the port-only command rule. Existing messenger claim/execution
uses two transactions; this slice preserves that boundary rather than promising
atomic delivery across a process crash.
