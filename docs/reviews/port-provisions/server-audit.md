# Server provision-source audit

Canon: Timothy 2026-10-28 — boats provision at their actual departure port. Support/upkeep anchors remain unchanged. No schema, rations, food selection, crew, event payload or return-route mechanics changed.

- Ordinary missions, pickup and passage initial dispatch: `combat.StartMarch` and `PreviewMarch` now resolve `economy.ShipDeparturePort`. Active owned same-world garrison city is required; stored coordinates, when present, must match. Legacy NULL coordinates retain settlement docking evidence. No harbour-building requirement added. Nil support no longer bypasses provisioning.
- Transfer, gift, bilateral trade and standing naval outbound: `transport` already validates `FreightPort` against actual OriginID, owner, world and position, then calls `economy.LoadProvisions`. Unchanged.
- Automatic/damaged freight returns sail on onboard food. They do not load remotely from the home reference. Unchanged.
- Physically arrived mission onward/return departures: `combat/return_food.go` resolves the reached town then uses `economy.HospitalityFood`; bilateral return hospitality uses `economy/ship_hospitality_transport.go`. Includes existing foreign-city hospitality. Unchanged.
- Standing-order direct `TakeFood` is in the land-only branch. Upkeep food consumption and all ration/ledger owners remain unchanged.

Red first: `/tmp/port-provisions-red.log` names `TestCityLandingAllPassengersStayAndSupportUnchanged`: empty physical port incorrectly permitted preview despite rich remote support.

Fresh full combat green: `/tmp/port-provisions-combat-final.log`. Fresh full economy and transport green: `/tmp/port-provisions-packages-final.log` (its combat failure was a subsequently corrected new test fixture). Parent owns final full-suite/native proofs.

Regression coverage follows real city landing then next voyage: physical stock depleted, remote stock unchanged, nil support still loads locally; insufficient physical food rejects preview and dispatch without changing ship/remote stock. `TestShipDeparturePortRequiresPhysicalOwnedActiveCity` covers foreign owner, contradictory position, at-sea status and missing physical settlement. `TestShipAtSeaOrderNeverLoadsRemoteSupportFood` exercises existing legal return to own port, proving aboard-only food during preview/dispatch. Existing mission fixture departures previously bypassed food with nil support; six such fixtures now provide real local food.

Limits: helper explicitly checks settlement/province world and active state, but those two predicates do not yet have individual mutants. Existing freight resolver behavior is reused unchanged; no new physical-eligibility rules inferred from this change.
