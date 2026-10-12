-- Fixed player thresholds become loyalty mode. Keep unrelated JSON and all
-- sealed letters / pending scheduled payloads / audit events unchanged.
UPDATE player_world_records
 SET retreat_default = CASE WHEN retreat_default->'hold_to_last_man' = 'true'::jsonb
   THEN retreat_default - 'retreat_at_loss' - 'by_loyalty'
   ELSE retreat_default - 'retreat_at_loss' - 'hold_to_last_man' - 'by_loyalty' END
 WHERE retreat_default ? 'retreat_at_loss';
UPDATE battle_participants
 SET standing_orders = CASE WHEN standing_orders->'hold_to_last_man' = 'true'::jsonb
   THEN standing_orders - 'retreat_at_loss' - 'by_loyalty'
   ELSE standing_orders - 'retreat_at_loss' - 'hold_to_last_man' - 'by_loyalty' END
 WHERE standing_orders ? 'retreat_at_loss';
