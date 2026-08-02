-- Rader raderade av upp-migrationen kommer inte tillbaka — down tar bara
-- bort FK-begränsningarna, inte de föräldralösa raderna (de är borta för gott).

ALTER TABLE trade_routes DROP CONSTRAINT trade_routes_world_id_fkey;
ALTER TABLE trade_routes
    ADD CONSTRAINT trade_routes_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id);

ALTER TABLE scheduled_events DROP CONSTRAINT scheduled_events_world_id_fkey;
ALTER TABLE player_scouted_tiles DROP CONSTRAINT player_scouted_tiles_world_id_fkey;
ALTER TABLE player_scouted_provinces DROP CONSTRAINT player_scouted_provinces_world_id_fkey;
ALTER TABLE loyalty_events DROP CONSTRAINT loyalty_events_world_id_fkey;
ALTER TABLE known_settlements DROP CONSTRAINT known_settlements_world_id_fkey;
ALTER TABLE gossip_events DROP CONSTRAINT gossip_events_world_id_fkey;
ALTER TABLE events DROP CONSTRAINT events_world_id_fkey;
ALTER TABLE build_queue DROP CONSTRAINT build_queue_world_id_fkey;
