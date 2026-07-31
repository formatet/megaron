-- 104: Ingen rad överlever sin värld.
--
-- Åtta världsskopade tabeller saknade helt en FOREIGN KEY mot worlds(id), så
-- varje reseed (cmd/create-world: `TRUNCATE worlds CASCADE`) hoppade rätt
-- över dem — TRUNCATE CASCADE följer bara FK-grafen, och dessa åtta stod
-- utanför den. Mätt på driftservern 2026-07-31: events bar 1 811 389 rader
-- varav 1 767 139 (97,6 %) tillhörde raderade världar, 854 MB av 1015 MB
-- databas. Ett städjobb måste köras om för varje ny tabell för alltid; en FK
-- gäller för alltid. Detta är den fixen, inte ett engångsstäd.
--
-- trade_routes hade redan en FK men med NO ACTION — den konverteras här till
-- CASCADE så den följer samma regel som alla andra världsskopade tabeller.
--
-- scheduled_events får sin FK här men ingen retention-städning: tabellen har
-- noll föräldralösa rader idag (bara historiskt kvarhållna behandlade rader),
-- en annan rot som hör till en egen framtida slice.
--
-- Diskutrymmet som frigörs av DELETE-satserna nedan syns inte förrän
-- autovacuum kör eller Timothy kör VACUUM FULL manuellt — går inte att göra
-- i en migrations-transaktion.

-- Städa föräldralösa rader FÖRST — FK kan inte skapas mot data som bryter den.
DELETE FROM build_queue WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM events WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM gossip_events WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM known_settlements WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM loyalty_events WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM player_scouted_provinces WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM player_scouted_tiles WHERE world_id NOT IN (SELECT id FROM worlds);
DELETE FROM scheduled_events WHERE world_id NOT IN (SELECT id FROM worlds);

ALTER TABLE build_queue
    ADD CONSTRAINT build_queue_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE events
    ADD CONSTRAINT events_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE gossip_events
    ADD CONSTRAINT gossip_events_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE known_settlements
    ADD CONSTRAINT known_settlements_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE loyalty_events
    ADD CONSTRAINT loyalty_events_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE player_scouted_provinces
    ADD CONSTRAINT player_scouted_provinces_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE player_scouted_tiles
    ADD CONSTRAINT player_scouted_tiles_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

ALTER TABLE scheduled_events
    ADD CONSTRAINT scheduled_events_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;

-- trade_routes: NO ACTION → CASCADE (samma regel som varje annan tabell).
ALTER TABLE trade_routes DROP CONSTRAINT trade_routes_world_id_fkey;
ALTER TABLE trade_routes
    ADD CONSTRAINT trade_routes_world_id_fkey
    FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;
