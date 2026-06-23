ALTER TABLE worlds ADD COLUMN status text NOT NULL DEFAULT 'active';
-- collapse current multi-active state down to a single active world:
UPDATE worlds SET status = 'archived';
UPDATE worlds SET status = 'active'
  WHERE id = (SELECT world_id FROM events ORDER BY created_at DESC LIMIT 1);
CREATE UNIQUE INDEX one_active_world ON worlds (status) WHERE status = 'active';
