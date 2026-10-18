-- New archive rows keep the tick of their own world at insertion. Old rows
-- are unknown unless the original payload explicitly recorded world_tick;
-- wall time cannot recover a tick across changing simulation cadences.
ALTER TABLE notifications ADD COLUMN world_tick INTEGER CHECK (world_tick >= 0);
UPDATE notifications
SET world_tick = (body_json->>'world_tick')::INTEGER
WHERE CASE WHEN body_json->>'world_tick' ~ '^[0-9]{1,10}$'
           THEN (body_json->>'world_tick')::BIGINT <= 2147483647
           ELSE false END;

CREATE FUNCTION capture_notification_world_tick() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.world_tick IS NULL THEN
    SELECT current_tick INTO NEW.world_tick FROM worlds WHERE id = NEW.world_id;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER notification_world_tick_before_insert
BEFORE INSERT ON notifications FOR EACH ROW
EXECUTE FUNCTION capture_notification_world_tick();
