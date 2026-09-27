-- Migration 152: "hämta hem" — skeppsuppdrag 2b (megaron_plan_hamta_hem.md).
--
-- A ship dispatched with march_intent='pickup' (and, once parked waiting off
-- the shore for the fetched unit, 'pickup_wait') carries the id of the unit
-- it was sent to fetch, and how many ticks it will wait there before turning
-- for home regardless. The chosen shore hex itself rides on the existing
-- land_target_q/land_target_r columns (mig 147) — boardShipMissions already
-- reads those generically for any ship mission, so no new column is needed
-- for that half. NULL for every ship not on a pickup mission — additive and
-- inert otherwise.
--
-- ON DELETE SET NULL: the fetched unit dissolving (disbanded/destroyed) while
-- the ship waits must not block that delete — HandlePickupTimeout's own guard
-- already treats a vanished pickup_unit_id as "nothing left to wait for" and
-- sends the ship home empty.
ALTER TABLE units
    ADD COLUMN pickup_unit_id UUID REFERENCES units(id) ON DELETE SET NULL,
    ADD COLUMN pickup_wait_ticks INT;
