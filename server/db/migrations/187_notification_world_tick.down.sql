DROP TRIGGER notification_world_tick_before_insert ON notifications;
DROP FUNCTION capture_notification_world_tick();
ALTER TABLE notifications DROP COLUMN world_tick;
