-- Migration 108: vadstället blir en egen terräng (megaron_plan_flodbudget_och_
-- vadstalle.md, Timothy 2026-08-02) — flodens port, seglingsbar OCH vadbar,
-- ett per ~10 längdhexar av en flods egen kedja.
--
--   1. Fisken: `river_ford` är vatten precis som `river` (mig 101, Timothy
--      2026-07-29: "allt vatten ger fisk"). Samma rate som `river` — läst UR
--      TABELLEN via SELECT nedan, aldrig hårdkodad eller kopierad ur ett
--      dokument (mig 071 skalade varenda production_rules-rate ×60 i förbifarten;
--      vinraterna stod fel i tre dygn av precis det skälet — CLAUDE.md/
--      megaron_arbetssatt.md §6). Om `river`s rate någonsin justeras UTAN att
--      denna rad skrivs om kommer den framtida migrationen att behöva besluta
--      om `river_ford` ska följa med — det beslutet hör inte hemma här.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit, requires_coastal)
SELECT 'river_ford', building_type, good_key, rate_per_tick, requires_deposit, requires_coastal
  FROM production_rules
 WHERE terrain_type = 'river' AND good_key = 'fish';

-- 2. Backfill av map_tiles.coastal / provinces.coastal för vadställesgrannar.
--    I dag en NO-OP (ingen värld har river_ford-hexar än — mapgen carvar dem
--    först efter denna migration är körd i en NY seed), men skriven för
--    korrektheten, samma resonemang som mig 101 §3: en värld som seedas om
--    med denna migration redan applicerad ska inte behöva en andra
--    backfill-omgång för hexar mapgen redan satte `coastal` på korrekt via
--    den uppdaterade hasWaterNeighbour (mapgen.go).
UPDATE map_tiles mt
   SET coastal = TRUE
  FROM map_tiles nb
 WHERE nb.world_id = mt.world_id
   AND nb.terrain = 'river_ford'
   AND mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')
   AND (nb.q, nb.r) IN (
       (mt.q+1, mt.r), (mt.q-1, mt.r),
       (mt.q, mt.r+1), (mt.q, mt.r-1),
       (mt.q+1, mt.r-1), (mt.q-1, mt.r+1)
   )
   AND mt.coastal = FALSE;

UPDATE provinces p
   SET coastal = TRUE
  FROM map_tiles mt
 WHERE mt.world_id = p.world_id AND mt.q = p.map_q AND mt.r = p.map_r
   AND mt.coastal = TRUE AND p.coastal = FALSE;

-- 3. INGEN retro-konvertering av befintlig river-terräng till river_ford i
--    levande världar. Precis som mig 101 §4: att konvertera terräng under en
--    aktiv spelares fötter (eller under en marscherande enhet, eller en stad
--    som redan räknar sin kuststatus) är inte en migration, det är
--    dataförlust. Vadstället är därför en RESEED-feature — Timothy avgör när
--    live-världen seedas om.
--
-- 4. INGEN backfill av settlement_goods behövs, samma resonemang som mig 101
--    §5: economy.RecomputeProduction körs varje tick och läser
--    production_rules LIVE.
