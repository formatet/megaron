-- A recalled expedition keeps its mission and its sight list and turns home, so the
-- homecoming files the usual ExpeditionReport. 'recalled' is the turn reason.
ALTER TABLE unit_expeditions DROP CONSTRAINT unit_expeditions_turn_reason_check;
ALTER TABLE unit_expeditions ADD CONSTRAINT unit_expeditions_turn_reason_check
    CHECK (turn_reason IN ('half_time', 'area_known', 'no_path', 'recalled'));
