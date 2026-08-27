-- Ner-migration för 136: återställ dagsverkesskalan till de gamla talen.
--
-- Ordningen är up:s spegelvända: base_value → settlement_goods → återskapa
-- gratisflöden → sänk byggnadsraderna → återställ farmbonusen → multiplicera
-- tillbaka production_rules.
--
-- ⚠️ Inte bit-exakt reversibel: up:ens settle-moment (steg 5) skriver amount till
-- vad den lata tupeln stod i vid omskalningstillfället och nollställer den
-- förflutna tiden sedan calc_tick. Den tiden går inte att återskapa. Mängderna
-- och takterna återställs korrekt; bara "tid sedan senaste beräkning" går förlorad,
-- vilket är samma egenskap mig 109 har.

-- ── 6. base_value tillbaka ─────────────────────────────────────────────────────
UPDATE goods SET base_value = base_value / 216   WHERE key = 'timber';
UPDATE goods SET base_value = base_value / 86.4  WHERE key = 'fish';
UPDATE goods SET base_value = base_value / 72    WHERE key = 'cedar';
UPDATE goods SET base_value = base_value / 43.2  WHERE key = 'grain';
UPDATE goods SET base_value = base_value / 36    WHERE key = 'livestock';
UPDATE goods SET base_value = base_value / 28.8  WHERE key = 'copper';
UPDATE goods SET base_value = base_value / 21.6  WHERE key = 'oil';
UPDATE goods SET base_value = base_value / 14.4  WHERE key = 'tin';
UPDATE goods SET base_value = base_value / 14.4  WHERE key = 'wine';
UPDATE goods SET base_value = base_value / 7.2   WHERE key = 'stone';
UPDATE goods SET base_value = base_value / 43.2  WHERE key = 'pottery';
UPDATE goods SET base_value = base_value / 28.8  WHERE key = 'horses';
UPDATE goods SET base_value = base_value / 21.6  WHERE key = 'purple';

-- ── 5b. settlements.food_unmet_amount tillbaka ────────────────────────────────
UPDATE settlements SET food_unmet_amount = food_unmet_amount * 43.2
 WHERE food_unmet_amount <> 0;

-- ── 5. settlement_goods tillbaka ───────────────────────────────────────────────
UPDATE settlement_goods sg
SET amount    = GREATEST(0, sg.amount + sg.rate * GREATEST(0, w.current_tick - sg.calc_tick))
                * d.divisor,
    rate      = sg.rate * d.divisor,
    calc_tick = w.current_tick
FROM settlements s
JOIN worlds w ON w.id = s.world_id
CROSS JOIN (VALUES
        ('timber', 216.0), ('fish', 86.4), ('cedar', 72.0), ('grain', 43.2),
        ('livestock', 36.0), ('copper', 28.8), ('oil', 21.6), ('tin', 14.4),
        ('wine', 14.4), ('stone', 7.2), ('pottery', 43.2), ('horses', 28.8),
        ('purple', 21.6)
     ) AS d(good_key, divisor)
WHERE s.id = sg.settlement_id AND d.good_key = sg.good_key;

-- ── 4. Gratisflödena åter ──────────────────────────────────────────────────────
-- Talen är de som stod i production_rules före 136: timmertrickeln från
-- mig 032/033 och purpurflödet. Båda ovillkorliga (ingen terräng, ingen byggnad).
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick)
VALUES (NULL, NULL, 'timber', 144), (NULL, NULL, 'purple', 21.6)
ON CONFLICT DO NOTHING;

-- ── 3. Byggnadsraderna tillbaka till sina gamla tal ────────────────────────────
-- ⚠️ Skrivna som gammalt_värde / divisor, INTE som det gamla absoluta talet:
-- steg 1 nedan multiplicerar varje rad med divisorn igen, så ett absolut tal här
-- hade blivit divisorn gånger för stort.
UPDATE production_rules SET rate_per_tick = 86.4 / 86.4 WHERE good_key = 'fish'   AND building_type = 'harbour';
UPDATE production_rules SET rate_per_tick = 144  / 72   WHERE good_key = 'cedar'  AND building_type = 'lumbermill';
UPDATE production_rules SET rate_per_tick = 57.6 / 28.8 WHERE good_key = 'copper' AND building_type = 'mine';
UPDATE production_rules SET rate_per_tick = 36   / 14.4 WHERE good_key = 'tin'    AND building_type = 'mine';
UPDATE production_rules SET rate_per_tick = 360  / 216  WHERE good_key = 'timber' AND building_type = 'lumbermill' AND terrain_type IS NULL;

-- ── 2. Farmens spannmålsbonus tillbaka till sina ursprungliga, spridda tal ─────
-- Kullar ×2,0 · slätt ×3,33 · floddal ×3,0 · flodmynning ×2,5 — skrivna som
-- absoluta tal eftersom spridningen inte går att uttrycka som en multiplikator.
-- (Värdena är de som stod i production_rules före 136, delade med 43,2 så de
-- passar in i den skala steg 1 nedan multiplicerar tillbaka.)
UPDATE production_rules SET rate_per_tick = 28.8 / 43.2  WHERE good_key = 'grain' AND terrain_type = 'hills'        AND building_type = 'farm';
UPDATE production_rules SET rate_per_tick = 144  / 43.2  WHERE good_key = 'grain' AND terrain_type = 'plains'       AND building_type = 'farm';
UPDATE production_rules SET rate_per_tick = 216  / 43.2  WHERE good_key = 'grain' AND terrain_type = 'river_valley' AND building_type = 'farm';
UPDATE production_rules SET rate_per_tick = 288  / 43.2  WHERE good_key = 'grain' AND terrain_type = 'river_delta'  AND building_type = 'farm';

-- ── 1. production_rules multipliceras tillbaka ─────────────────────────────────
UPDATE production_rules SET rate_per_tick = rate_per_tick * 216   WHERE good_key = 'timber';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 86.4  WHERE good_key = 'fish';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 72    WHERE good_key = 'cedar';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 43.2  WHERE good_key = 'grain';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 36    WHERE good_key = 'livestock';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 28.8  WHERE good_key = 'copper';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 21.6  WHERE good_key = 'oil';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 14.4  WHERE good_key = 'tin';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 14.4  WHERE good_key = 'wine';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 7.2   WHERE good_key = 'stone';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 43.2  WHERE good_key = 'pottery';
UPDATE production_rules SET rate_per_tick = rate_per_tick * 28.8  WHERE good_key = 'horses';
