-- Migration 101: floden blir en vattenväg (megaron_floden_plan.md S1, Timothy 2026-07-29).
--
-- `river` blir ett eget vattenterräng-värde (server/internal/world/model.go
-- TerrainRiver). Denna migration bär de DB-sidiga konsekvenserna:
--
--   1. Fisken flyttar FRÅN land TILL vatten. Fram tills nu kom fish från en
--      landregel (terrain_type NULL, requires_coastal = TRUE) — varje land-hex
--      som RÅKAR gränsa till coastal_sea gav fisk, medan själva vattnet gav
--      NOLL (economy/recompute.go m.fl. utesluter uttryckligen deep_sea och
--      coastal_sea från catchment-joinen). Timothys beslut: allt vatten ger
--      fisk. De nya reglerna är nycklade på VATTNETS EGEN terräng, inte på
--      granne-flaggan, och matchar bara via den nya OR-klausulen i Go-koden
--      (mt.terrain NOT IN (...) OR pr.terrain_type = mt.terrain) — utan den
--      klausulen skulle en vattenhex matcha VARJE terrain_type IS NULL-regel
--      (timmer, sten via mine, keramik via market), vilket är precis den tysta
--      fallback-produktionen CLAUDE.md förbjuder.
--
--   2. Rangordning grunt hav > flod > djuphav (Timothys ord: djuphavet ska
--      vara magert — öppet hav fiskas inte från stranden). Talen är kalibrerade
--      mot en verklig kuststads fisk/tick i acceptansvärlden så att en typisk
--      kuststad rör sig ≤25 % mot dagens tal (A4 i planen) — se
--      megaron_floden_plan.md §Acceptanskriterier. 2.4/tick var den gamla
--      raten per matchande (hex, regel)-par (0.04/min × 60, mig 071).
--         coastal_sea (bas)      2.4   — samma bas som den gamla landregeln
--         coastal_sea (harbour)  2.4   — samma bas, nu nycklat på själva vattnet
--         river                  1.2   — grunt och skyddat, men smalt: hälften
--         deep_sea               0.4   — magert öppet hav, en sjättedel
DELETE FROM production_rules
 WHERE good_key = 'fish' AND requires_coastal = TRUE AND terrain_type IS NULL;

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('coastal_sea', NULL,      'fish', 2.4, NULL),
    ('coastal_sea', 'harbour', 'fish', 2.4, NULL),
    ('river',       NULL,      'fish', 1.2, NULL),
    ('deep_sea',    NULL,      'fish', 0.4, NULL);

-- 3. Backfill map_tiles.coastal / provinces.coastal för flodgrannar. I dag en
--    NO-OP (ingen värld har river-hexar än — mapgen carvar dem först efter
--    denna migration är körd i en NY seed), men skriven för korrektheten: en
--    värld som seedas om med denna migration redan applicerad ska inte behöva
--    en andra backfill-omgång för hexar mapgen redan satte `coastal` på
--    korrekt via den nya hasWaterNeighbour (mapgen.go).
UPDATE map_tiles mt
   SET coastal = TRUE
  FROM map_tiles nb
 WHERE nb.world_id = mt.world_id
   AND nb.terrain = 'river'
   AND mt.terrain NOT IN ('deep_sea','coastal_sea','river')
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

-- 4. INGEN retro-konvertering av befintliga river_valley-kedjor till river.
--    En levande värld kan ha en stad, enhet eller marsch stående på en hex som
--    skulle bli vatten under den nya modellen — att konvertera terräng under
--    en aktiv spelares fötter är inte en migration, det är dataförlust. Floden
--    är därför en RESEED-feature: Timothy avgör när live-världen seedas om
--    (samma resonemang som mig 092's NOTE om att inte bakåträkna befintligt
--    tillstånd).
--
-- 5. INGEN backfill av settlement_goods behövs. economy.RecomputeProduction
--    körs varje tick ur kharis/tick.go och läser production_rules LIVE — nästa
--    tick efter denna migration räknar redan om med de nya fiskereglerna,
--    exakt samma resonemang som mig 092's NOTE.
