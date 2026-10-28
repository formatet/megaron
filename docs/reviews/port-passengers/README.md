# Embarked cohort docks with its ship

Contract: close todo's missing passenger regression for the harbour fix. Test-only scope: an embarked cohort sails to another owned port, remains aboard its original ship, and has that port's settlement and city coordinates before the ship departs again. Supports the physical-order learning chain; no rule changes.

Fresh DB migration 224: targeted test green. Mutant disables arriveGarrison's embarked-cohort update using AND false: named assertion fails because the cohort retains its former settlement. Original source restored byte-for-byte. Full fresh suite recorded separately before integration.

Test: TestStartMarch_NavalPlainMarchToFriendlyPortDocks. No new player surface or asset.

Full fresh Go suite passed (world 154.349s); combat vet passed. No production files changed.
