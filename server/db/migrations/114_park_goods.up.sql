-- Migration 114: park purple/pottery/horses (Temenos_varutaxonomi_sol.md §4.2).
-- status column on goods: 'active' | 'parked'. Parked = kept in data/lore,
-- gated out of production, labor allocation and trade offers by every reader
-- that joins production_rules or lists tradeable goods (see recompute.go,
-- catchment.go, catchment_preview.go, province.go LaborAlloc/Goods,
-- combat/build.go AutoAllocateUnlocked trigger, api/handlers/goods.go).
-- cult and luxury are untouched here — cult is an FK-anchor for
-- settlement_labor.good_key (megaron_cult_ar_ingen_vara_plan.md) and cannot be
-- parked; luxury's avveckling is a separate open decision (S4, plan
-- megaron_plan_varukatalogen.md).
ALTER TABLE goods
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'parked'));

-- Purple — parkerad: saknar aktiv prestige- och hovekonomi.
-- Återkomstvillkor: hov, regalier, diplomati, legitimitet.
UPDATE goods SET status = 'parked' WHERE key = 'purple';

-- Pottery — parkerad: riskerar lager utan beslut.
-- Återkomstvillkor: hushåll, magasin, olja/vintransport.
UPDATE goods SET status = 'parked' WHERE key = 'pottery';

-- Horses — parkerad: får inte produceras abstrakt ur stable.
-- Återkomstvillkor: avel, foder, vagnsförluster, budbärare.
UPDATE goods SET status = 'parked' WHERE key = 'horses';
