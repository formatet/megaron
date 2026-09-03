-- Migration 139: Metallpriser — tenn får sin premie tillbaka, brons prissätts
-- för första gången.
--
-- Kontext (kodverifierat 2026-09-02): goods.base_value är pris per enhet, läst
-- av internal/religion/index.go (DivineValue → orakelodds och gudarnas
-- värdering av offer), varukatalogen (api/handlers/province.go, GET .../goods)
-- och economy.GoodBaseValue. Mig 136 (dagsverkesskalan) multiplicerade varje
-- varas base_value med dess GAMLA produktionstakt för att hålla värdet per
-- DAGSVERKE oförändrat — konsekvent i sin egen ram, men den prissätter arbete,
-- inte knapphet. Två fel följde:
--
-- (1) Tennets premie försvann. Före 136: koppar 6, tenn 12 — tenn dubbelt så
-- värdefullt. Efter 136 stod båda på 172,8, eftersom koppar och tenn råkade ha
-- samma gamla produktionstakt (28,8 resp. 14,4 — divisorerna skiljer, men
-- kvoten base_value(gammal)/divisor landade lika). Tennets knapphet är
-- geologisk (mapgen: copperSourceTarget skalar med spelarantalet, tinSourceCap
-- är ett tak) — kartan vet att tenn är knappt, ekonomin visste det inte.
--
-- (2) Brons prissattes ALDRIG. `grep bronze db/migrations/136_dagsverkesskalan.up.sql`
-- ger noll träffar: brons görs ur ett recept (recipes/recipe_ingredients), inte
-- ur en hex, så det hade ingen produktionstakt att skala och ingen prissatte
-- utfallet efteråt. Receptet (mig 010, kvantitet ändrad av mig 099) är
-- 9 koppar + 1 tenn → 1 brons (foundry, output_qty=1,0) — verifierat mot
-- poleia_test_metall (fräsch DB, migrerad till 137) innan denna migration
-- skrevs. Med koppar/tenn på 172,8/172,8 kostade receptet 1 728 + 172,8 =
-- 1 900,8 i ingrediensvärde medan brons stod kvar på sitt ursprungliga 20
-- (mig 008) — smältning "förstörde" 92 % av värdet.

-- ── Tennpremie ──────────────────────────────────────────────────────────────
-- Tenn = 2× koppar (relationen som gällde före mig 136). Koppar är ankaret och
-- rörs inte — att flytta båda hade rört fler förhållanden än nödvändigt.
-- 172,8 × 2 = 345,6. Skrivet som en subquery mot kopparns LIVE base_value,
-- inte hårdkodat 345,6, så relationen håller även om koppar någonsin ändras.
UPDATE goods SET base_value = (SELECT base_value FROM goods WHERE key = 'copper') * 2
WHERE key = 'tin';

-- ── Bronspris, härlett ur receptet ───────────────────────────────────────────
-- Recept (recipes ⋈ recipe_ingredients, output_key='bronze',
-- building_type='foundry', output_qty=1,0): 9 koppar + 1 tenn.
--
-- Ingrediensvärde med de NYA talen (koppar 172,8 oförändrad ovan, tenn nu
-- 345,6 från steget precis före detta i samma migration):
--   9 koppar × 172,8  = 1 555,2
--   1 tenn   × 345,6  =   345,6
--   ──────────────────────────
--   summa              = 1 900,8
--
-- Marginal ×1,5 (Timothy: smältning ska löna sig tydligt — brons är kedjans
-- slutsteg och kräver dessutom en sjöresa till en kustlös spelare — men
-- marginalen är en REGEL för hela ekonomin, inte ett hittepåtal för just
-- brons):
--   1 900,8 × 1,5 = 2 851,2
--
-- Räknat av SQL:en nedan (SUM(ri.quantity * g.base_value) över receptets
-- rader, läst live), inte hårdkodat till 2851,2 — samma skäl
-- internal/economy/recompute.go läser samma recept live i stället för att anta
-- 9:1: värdet kan aldrig drifta isär från receptet det bygger på.
UPDATE goods SET base_value = (
    SELECT SUM(ri.quantity * g.base_value) * 1.5
    FROM recipes r
    JOIN recipe_ingredients ri ON ri.recipe_id = r.id
    JOIN goods g ON g.key = ri.good_key
    WHERE r.output_key = 'bronze' AND r.building_type = 'foundry'
)
WHERE key = 'bronze';

-- ⛔ SILVER RÖRS INTE — valutans ankare (numéraire), base_value = 1, oförändrat.
--
-- ⛔ Startsilvret (economy.GenesisSilverLiquid / dailyGrainNeedInSilver) prisas
-- ENDAST mot grain.base_value — economy.GoodBaseValue anropas i hela kodbasen
-- bara med "grain" (verifierat med grep över api/handlers och internal/combat).
-- Denna migration rör varken grain eller silver, så startsilvret är opåverkat.
