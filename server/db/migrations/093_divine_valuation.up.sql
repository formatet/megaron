-- 093: Gudarnas värdering — brist i VÄRLDEN, inte i staden.
--
-- economy.LocalPrice mäter brist lokalt: stadens lager mot dess egen
-- produktionsreferens. Gudarna dömer annorlunda — de ser hela världen.
-- "Dödliga prissätter efter sitt eget behov, gudarna efter världens."
-- (Timothy 2026-07-22, temenos_prayers_komposition_plan.md)
--
-- Två knappheter, avsiktligt åtskilda:
--   spread — hur FÅ Wanaxer som håller varan (monopol, handelsvärde)
--   volume — hur LITE som finns totalt (ren sällsynthet)
-- En vara många har lite av är inte samma sak som en vara EN Wanax har berg av.

-- Religiöst kodade varor: det gudarna faktiskt tar emot, och det enda tiondet
-- rör. wine/oil är redan tempeloffret (kharis.applyTempleOffering); purple och
-- luxury är prestigevaror vars enda rimliga sänka är gudarna och de döda.
ALTER TABLE goods ADD COLUMN IF NOT EXISTS religious BOOLEAN NOT NULL DEFAULT false;
UPDATE goods SET religious = true WHERE key IN ('wine', 'oil', 'purple', 'luxury');

-- Gudarnas prislista, omräknad på dygns-ticken. ALDRIG aggregerad per rit —
-- en bön får inte kosta en världsomspännande scan.
CREATE TABLE IF NOT EXISTS divine_valuations (
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    good_key      TEXT NOT NULL REFERENCES goods(key),
    -- 0..1, högt = få håller varan
    rarity_spread DOUBLE PRECISION NOT NULL DEFAULT 0,
    -- 0..1, högt = lite finns i världen (log-dämpad, annars dominerar grain allt)
    rarity_volume DOUBLE PRECISION NOT NULL DEFAULT 0,
    -- base_value × (1 + K·rarity), utjämnad mot föregående dygn så gudarnas
    -- smak rör sig men inte rycker.
    divine_value  DOUBLE PRECISION NOT NULL DEFAULT 0,
    calc_tick     BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (world_id, good_key)
);

CREATE INDEX IF NOT EXISTS idx_divine_valuations_world ON divine_valuations(world_id);
