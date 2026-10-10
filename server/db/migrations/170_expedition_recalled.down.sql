UPDATE unit_expeditions SET turn_reason = 'half_time' WHERE turn_reason = 'recalled';
ALTER TABLE unit_expeditions DROP CONSTRAINT unit_expeditions_turn_reason_check;
ALTER TABLE unit_expeditions ADD CONSTRAINT unit_expeditions_turn_reason_check
    CHECK (turn_reason IN ('half_time', 'area_known', 'no_path'));
