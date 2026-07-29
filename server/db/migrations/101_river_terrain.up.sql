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
--      vara magert — öppet hav fiskas inte från stranden). Talen är MÄTTA, inte
--      gissade (A4 i planen, megaron_floden_plan.md §Acceptanskriterier):
--      2.4/tick var den gamla raten per matchande (land, regel)-par (0.04/min
--      × 60, mig 071) — en kuststad fick 2.4 för VARJE land-hex i sin
--      catchment som RÅKADE gränsa till coastal_sea, oavsett hur många
--      faktiska sjöhexar den gränsade. En första gissning (coastal_sea=2.4,
--      samma tal) mättes mot alla kustnära bebyggbara hexar i en seedad
--      acceptansvärld (30×20, engångsanalys — se processrapporten/slutrapporten
--      för slicen) och gav en median-ratio klart under 1 (typisk kuststad
--      TAPPAR fisk) — antalet land-hexar som råkar gränsa till hav är så gott
--      som alltid FLER än antalet faktiska sjöhexar i en 7-hex-catchment.
--      coastal_sea=3.6 centrerade om ~1,0 och gav flest hexar innanför ±25 %
--      av åtta testade nivåer (2.4 upp till 12.0). river/deep_sea skalade
--      proportionellt mot samma bas (0,5× / 1/6×) för att hålla rangordningen:
--         coastal_sea (bas)      3.6
--         coastal_sea (harbour)  3.6   — samma bas, nu nycklat på själva vattnet
--         river                  1.8   — grunt och skyddat, men smalt: hälften
--         deep_sea               0.6   — magert öppet hav, en sjättedel
--      Spridningen mellan enskilda hexar är stor (0,08×–6,00× vid bästa
--      raten) eftersom modellerna räknar helt olika saker — se känd
--      avgränsning i processrapporten.
DELETE FROM production_rules
 WHERE good_key = 'fish' AND requires_coastal = TRUE AND terrain_type IS NULL;

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('coastal_sea', NULL,      'fish', 3.6, NULL),
    ('coastal_sea', 'harbour', 'fish', 3.6, NULL),
    ('river',       NULL,      'fish', 1.8, NULL),
    ('deep_sea',    NULL,      'fish', 0.6, NULL);

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
