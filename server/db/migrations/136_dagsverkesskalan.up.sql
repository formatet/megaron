-- Migration 136: Dagsverkesskalan — en gubbe på standardterräng producerar 1/tick.
--
-- Beslut låst 2026-08-27 (Timothy): "att mängden vi utgår från är att EN GUBBE på
-- en standardyta för den varan producerar 1 per tick". Då blir varje kostnadstal
-- direkt läsbart som ett antal dagsverken. Plan: megaron_plan_dagsverkesskalan.md.
-- Mätunderlag: megaron_dagsverkesskalan.md.
--
-- ERSÄTTER megaron_plan_sadesskalan (÷100 för matvaror). Det talet var godtyckligt;
-- detta är principiellt. Grenen ekonomi/sadesskalan är parkerad — merga den ALDRIG
-- ovanpå denna, då delas grain med 100 och sedan med 43,2 igen.
--
-- ── Modellen ───────────────────────────────────────────────────────────────────
-- placementYield = (rate_per_tick / capL1) × mult × placerade gubbar
-- alltså: rate_per_tick är TOTALEN för en fullbemannad hex på byggnadsnivå 1,
-- inte per gubbe. En ren division av rate med varans divisor ger därför per-gubbe
-- = 1,00 på standardytan och bevarar ALLA relativa förhållanden mellan terränger.
--
-- ⭐ Grain är undantaget, och det gör det enklare: hexGoodCaps pinnar grains
-- capL1=1, mult=1.0 och placeCap=cap, så grains rate_per_tick ÄR per-gubbe-talet
-- rakt av. Specialfallet rörs INTE — det skyddar ett hårt invarianttest (en
-- försummad startstad med exakt en odlingsbar hex får aldrig svälta).
--
-- ── Divisorer (dagens per-gubbe-takt på varans bästa yta utan byggnad) ─────────
--   timmer 216 · fisk 86,4 · ceder 72 · grain 43,2 · boskap 36 · koppar 28,8
--   olja 21,6 · tenn 14,4 · vin 14,4 · sten 7,2 · keramik 43,2 · hästar 28,8
--
-- ⛔ SILVER OCH BRONS RÖRS INTE. Brons har redan per-gubbe 1,0 (gjuteriet), så
-- dess divisor är 1. Silver är en VALUTA, inte en produktionsvara i samma mening:
-- lagret (startsilver 480, kassor ~30 000), soldnivåerna och byggkostnaderna är
-- kalibrerade mot varandra. Att skala om valutan är en egen systemfråga
-- (kollapsmekaniken, planens §5) och hör inte hemma i en produktionsnormalisering.
-- En slice = en sak.

-- ── 1. production_rules — ren division per vara ────────────────────────────────
UPDATE production_rules SET rate_per_tick = rate_per_tick / 216   WHERE good_key = 'timber';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 86.4  WHERE good_key = 'fish';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 72    WHERE good_key = 'cedar';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 43.2  WHERE good_key = 'grain';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 36    WHERE good_key = 'livestock';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 28.8  WHERE good_key = 'copper';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 21.6  WHERE good_key = 'oil';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 14.4  WHERE good_key = 'tin';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 14.4  WHERE good_key = 'wine';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 7.2   WHERE good_key = 'stone';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 43.2  WHERE good_key = 'pottery';
UPDATE production_rules SET rate_per_tick = rate_per_tick / 28.8  WHERE good_key = 'horses';

-- ── 2. Farmens spannmålsbonus sänks från ×3,33 till ×1,70 ──────────────────────
-- Steg 1 bevarar dagens farm-bonus (plains 43,2 → 144, alltså ×3,33). Kombinerat
-- med platstrappan (4 gubbar utan farm → 8 med farm nivå 1, låst 2026-08-22) ger
-- det 8 × 3,33 = 26,6 per hex mot 4,0 utan farm — 6,7× på ett byggsteg, och
-- Mochlos hade landat på ~250 grain/tick mot en konsumtion på 56.
--
-- ×1,70 är kalibreringsvalet (megaron_plan_dagsverkesskalan §3): farmen ska
-- FRIGÖRA arbetskraft, inte upphäva maten som begränsning. Totalen per hex blir
-- 8 × 1,70 = 13,6 mot 4,0 — en dryg tredubbling, buren av BÅDE fler platser och
-- högre takt. Mätt på Mochlos: 112,7 grain/tick mot 56 i konsumtion, alltså
-- +56,7 och 61 av 112 gubbar fria (megaron_plan_dagsverkesskalan §1.2).
--
-- Samma multiplikator på varje grain-terräng, till skillnad från idag där farmen
-- ger ×2,0 på kullar, ×3,33 på slätt, ×3,0 i floddal och ×2,5 i flodmynning —
-- en spridning ingen fattat beslut om.
--
-- ⚠️ Nivåtrappan för TAKTEN (planens kandidat 1,70/1,85/2,00 per farmnivå) byggs
-- INTE: production_rules har ingen nivåkolumn, och att införa en är ny mekanik.
-- Trappan kommer i stället från platserna (8/10/12 gubbar per farmnivå), vilket
-- ger 13,6 / 17,0 / 20,4 per hex. Nära kandidatens 13,6/18,5/24 utan nytt bygge.
UPDATE production_rules pr
SET rate_per_tick = bas.rate_per_tick * 1.70
FROM production_rules bas
WHERE pr.good_key = 'grain' AND pr.building_type = 'farm'
  AND bas.good_key = 'grain' AND bas.building_type IS NULL
  AND bas.terrain_type = pr.terrain_type;

-- ── 3. Ingen byggnad får sänka avkastningen per gubbe ──────────────────────────
-- Idag gör de det för halva katalogen: fisk 86,4 → 21,6 med hamn, ceder 72 → 36
-- med sågverk, koppar 28,8 → 11,52 med gruva, tenn 14,4 → 7,2 med gruva. Orsaken
-- är att byggnaden höjer hexens capL1 (capWithBuilding + WorkplaceSlots) medan
-- rate_per_tick inte följer med — så den TOTALA avkastningen stiger om du fyller
-- platserna, men varje enskild gubbe blir mindre värd. Det är involution, inte
-- intensifiering, och det gör byggnaden till ett dåligt beslut för en stad som
-- inte har folk att fylla den med.
--
-- Regeln efter denna migration: en byggnadsrad ger 1,70 per gubbe där
-- terrängraden ger 1,00 — samma bonus som farmen, av samma skäl.
--
-- capL1 med byggnad = capWithBuilding + WorkplaceSlots(byggnad, 1):
--   hamn     fisk    2 + 2 = 4     sågverk  timmer/ceder  2 + 2 = 4
--   gruva    metall  3 + 2 = 5     silvergruva            3 + 2 = 5
UPDATE production_rules SET rate_per_tick = 1.70 * 4
  WHERE good_key = 'fish'   AND building_type = 'harbour';
UPDATE production_rules SET rate_per_tick = 1.70 * 4
  WHERE good_key = 'cedar'  AND building_type = 'lumbermill';
UPDATE production_rules SET rate_per_tick = 1.70 * 5
  WHERE good_key = 'copper' AND building_type = 'mine';
UPDATE production_rules SET rate_per_tick = 1.70 * 5
  WHERE good_key = 'tin'    AND building_type = 'mine';
-- Sågverkets TIMMER-rad är terränglös (byggnadsväg via
-- LoadBuildingProductionOptions), så dess capL1 är WorkplaceSlots("lumbermill",1)
-- = 2. Ren division gav 360/216 = 1,667 totalt, alltså 0,83 per gubbe — LÄGRE än
-- olivlundens 1,00 utan byggnad. Samma involution som hamnen och gruvorna, bara
-- via den andra kodvägen. Fångad av TestProductionRules_NoBuildingLowersPerGubbe.
UPDATE production_rules SET rate_per_tick = 1.70 * 2
  WHERE good_key = 'timber' AND building_type = 'lumbermill' AND terrain_type IS NULL;

-- ── 4. Gratisflödena bort (Timothy 2026-08-27) ─────────────────────────────────
-- "vi tar bort avkastning för själva city-catchmenten förutom att den avkastar
-- mat som äts av en gubbe på en tick, varken mer eller mindre."
--
-- Varje stad fick per tick, utan att en enda gubbe arbetade: 144 timmer (0,67
-- dagsverken) och 21,6 purpur (1,0 dagsverke). Det är därför oljelagret stod på
-- 57 951 och timmerlagret på 14 000–50 000 medan NOLL gubbar arbetade med dem.
-- Timmertrickeln var ett deadlock-skydd från mig 032/033 ("en stad som gjort slut
-- på startpaketets timmer innan sågverket står ska inte fastna") som följde med
-- genom varje tidsomskalning sedan dess och blev större än allt annat i systemet.
--
-- Stadshexens spannmål är INTE ett sånt flöde och rörs inte här: det motsvarar
-- exakt en gubbes dagsranson och skalas i Go (NearjordGrainPerTick, se
-- internal/economy/recompute.go).
DELETE FROM production_rules
 WHERE good_key = 'timber' AND terrain_type IS NULL AND building_type IS NULL;
DELETE FROM production_rules
 WHERE good_key = 'purple' AND terrain_type IS NULL AND building_type IS NULL;

-- ── 5. settlement_goods — SETTLA FÖRE OMSKALNING ───────────────────────────────
-- Mönstret är mig 109:s och mig 136-utkastets: sätt amount till vad den lata
-- tupeln FAKTISKT står i vid GAMLA takten, i SAMMA statement som takten skalas
-- om — annars räknas hela den förflutna tiden om med fel takt och staden får
-- (eller mister) hela skillnaden.
--
-- Silver och brons har divisor 1 och utelämnas (se huvudkommentaren).
UPDATE settlement_goods sg
SET amount    = GREATEST(0, sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick))
                / d.divisor,
    rate      = sg.rate / d.divisor,
    calc_tick = w.current_tick
FROM settlements s
JOIN worlds w ON w.id = s.world_id
CROSS JOIN (VALUES
        ('timber', 216.0), ('fish', 86.4), ('cedar', 72.0), ('grain', 43.2),
        ('livestock', 36.0), ('copper', 28.8), ('oil', 21.6), ('tin', 14.4),
        ('wine', 14.4), ('stone', 7.2), ('pottery', 43.2), ('horses', 28.8),
        ('purple', 21.6)
     ) AS d(good_key, divisor)
-- CROSS JOIN + WHERE, inte JOIN ... ON: en UPDATE:s måltabell (sg) får inte
-- refereras i FROM-klausulens egna join-villkor i PostgreSQL.
WHERE s.id = sg.settlement_id AND d.good_key = sg.good_key;

-- ── 5b. settlements.food_unmet_amount (mig 134) ────────────────────────────────
-- Bär gårdagens obetäckta matbehov i SAMMA enhet som grain (Utfodringsordningen
-- D4 gatar svältgrenen på den). En vanlig kolumn, ingen lat tupel — ren division.
UPDATE settlements SET food_unmet_amount = food_unmet_amount / 43.2
 WHERE food_unmet_amount <> 0;

-- ── 6. goods.base_value — priset per enhet följer enheten ──────────────────────
-- En enhet är nu N gånger större än förut (N = varans divisor), så priset per
-- enhet måste upp med samma faktor för att en given MÄNGD ska vara värd lika
-- mycket. Utan detta blir varje vara ~N gånger undervärderad mot silver, och
-- grundningens startsilver (låst vid 480, economy.GenesisSilverLiquid prisar en
-- stads dagliga matbehov via grain.base_value) skulle tyst kollapsa.
--
-- ⭐ Detta är samma fälla sädesskalan gick i, fast åt andra hållet: DEN skalade
-- bara grain och lämnade fisk/boskap/vin/olja, så grain blev 60–150× mer värd per
-- enhet i DivineValue (internal/religion/index.go) utan spelmässigt skäl. Här
-- skalas ALLA varor med sin egen divisor, så de inbördes förhållandena — och
-- därmed gudarnas värdering och orakeloddsen — står exakt still.
UPDATE goods SET base_value = base_value * 216   WHERE key = 'timber';
UPDATE goods SET base_value = base_value * 86.4  WHERE key = 'fish';
UPDATE goods SET base_value = base_value * 72    WHERE key = 'cedar';
UPDATE goods SET base_value = base_value * 43.2  WHERE key = 'grain';
UPDATE goods SET base_value = base_value * 36    WHERE key = 'livestock';
UPDATE goods SET base_value = base_value * 28.8  WHERE key = 'copper';
UPDATE goods SET base_value = base_value * 21.6  WHERE key = 'oil';
UPDATE goods SET base_value = base_value * 14.4  WHERE key = 'tin';
UPDATE goods SET base_value = base_value * 14.4  WHERE key = 'wine';
UPDATE goods SET base_value = base_value * 7.2   WHERE key = 'stone';
UPDATE goods SET base_value = base_value * 43.2  WHERE key = 'pottery';
UPDATE goods SET base_value = base_value * 28.8  WHERE key = 'horses';
UPDATE goods SET base_value = base_value * 21.6  WHERE key = 'purple';
