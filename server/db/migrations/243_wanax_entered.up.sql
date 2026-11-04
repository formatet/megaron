-- WanaxEntered: when a Wanax founds their first city, every other Wanax in the
-- world is told (name + the founder's own message, never the place). One
-- founding = at most one such notice per recipient.
CREATE UNIQUE INDEX uq_notifications_wanax_entered
    ON notifications (world_id, player_id, (body_json->>'wanax_id'))
    WHERE kind = 'WanaxEntered';
