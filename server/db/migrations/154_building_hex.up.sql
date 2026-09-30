-- Migration 154: production buildings stand on a hex, not just in the city.
-- megaron_plan_byggnad_pa_hex.md §A (Timothy 2026-09-28): farm, mine,
-- lumbermill and stonequarry are hex-bound — each is placed on ONE catchment
-- hex and only affects THAT hex's production; a settlement may build several
-- of the same type, one per hex. Every other building (harbour, market,
-- wall, shipyard, foundry, stable, temple, olive_press, winery, barracks)
-- keeps hex_q/hex_r NULL — a city building, unchanged.
--
-- silver_mine is retired in the same slice: a `mine` built on a
-- silver-deposit hex now produces silver, exactly as a mine on a
-- copper/tin hex produces copper/tin. Cost and build time were already
-- identical to `mine` (province/building.go) — no balance change.
--
-- Beslut 5 (Timothy 2026-09-28): existing worlds get NO special-case
-- migration care beyond leaving the DB valid — there are no players on them.
-- Every existing hex-bound row is backfilled onto the settlement's OWN hex
-- (not a real production tile, but a valid coordinate) rather than the
-- catchment hex it may once have implicitly meant; anything that still can't
-- resolve a hex is simply deleted.

ALTER TABLE buildings ADD COLUMN hex_q INT;
ALTER TABLE buildings ADD COLUMN hex_r INT;
ALTER TABLE build_queue ADD COLUMN hex_q INT;
ALTER TABLE build_queue ADD COLUMN hex_r INT;

-- Fold silver_mine into mine (both the catalog rows and any built/queued
-- instances) BEFORE anything else touches building_type.
UPDATE production_rules SET building_type = 'mine' WHERE building_type = 'silver_mine';
UPDATE buildings SET building_type = 'mine' WHERE building_type = 'silver_mine';
UPDATE build_queue SET building_type = 'mine' WHERE building_type = 'silver_mine';

-- A settlement that had BOTH mine and silver_mine now has two 'mine' rows —
-- collapse to one per settlement (the higher level survives, tie-broken by
-- id) before the new unique index below can see them. No balance
-- consideration (beslut 5): this is a same-type merge, not a design choice.
DELETE FROM buildings
WHERE id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (
            PARTITION BY settlement_id, building_type
            ORDER BY level DESC, id
        ) AS rn
        FROM buildings
        WHERE building_type = 'mine'
    ) ranked
    WHERE ranked.rn > 1
);

-- Backfill hex-bound rows onto the settlement's own hex.
UPDATE buildings b
SET hex_q = prov.map_q, hex_r = prov.map_r
FROM settlements s
JOIN provinces prov ON prov.id = s.province_id
WHERE b.settlement_id = s.id
  AND b.building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry');

UPDATE build_queue bq
SET hex_q = prov.map_q, hex_r = prov.map_r
FROM settlements s
JOIN provinces prov ON prov.id = s.province_id
WHERE bq.settlement_id = s.id
  AND bq.building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry');

-- Anything that still couldn't resolve a hex (orphaned settlement/province —
-- should not happen under the existing FKs, kept defensive) is dropped
-- rather than left to violate the CHECK constraint below.
DELETE FROM buildings WHERE building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NULL;
DELETE FROM build_queue WHERE building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NULL;

ALTER TABLE buildings ADD CONSTRAINT buildings_hex_bound_check CHECK (
    (building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NOT NULL AND hex_r IS NOT NULL)
    OR (building_type NOT IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NULL AND hex_r IS NULL)
);
ALTER TABLE build_queue ADD CONSTRAINT build_queue_hex_bound_check CHECK (
    (building_type IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NOT NULL AND hex_r IS NOT NULL)
    OR (building_type NOT IN ('farm', 'mine', 'lumbermill', 'stonequarry') AND hex_q IS NULL AND hex_r IS NULL)
);

-- Replace the single (settlement_id, building_type) unique index with two
-- partial ones — a city building is still unique per type, a hex-bound
-- building is unique per (type, hex).
DROP INDEX IF EXISTS buildings_settlement_building;
CREATE UNIQUE INDEX buildings_settlement_building_city
    ON buildings (settlement_id, building_type) WHERE hex_q IS NULL;
CREATE UNIQUE INDEX buildings_settlement_building_hex
    ON buildings (settlement_id, building_type, hex_q, hex_r) WHERE hex_q IS NOT NULL;
