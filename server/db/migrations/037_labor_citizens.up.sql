-- Migration 037: labor_citizens
--
-- Byter settlement_labor.weight (normaliserad bråkdel) mot citizens (INT).
-- Formeln ändras från:
--   rate(g) = base_potential(g) × weight(g) × labor_pool / REF_LABOR
-- till:
--   rate(g) = yield_per_worker(g) × citizens(g)
--   yield_per_worker(g) = base_potential(g) / REF_LABOR
--
-- Σ citizens ≤ labor_pool; överskott = idle_citizens (inga fel, bara oallokerade).
-- Befintliga weight-rader migreras: citizens = ROUND(weight × labor_pool).
-- labor_pool = max(0, population − army_pop_cost − transit_pop_cost);
-- för migreringsändamål läses population från settlements och army-kolumner.

-- Lägg till citizens-kolumn och behåll weight tillfälligt för migreringsberäkning.
ALTER TABLE settlement_labor ADD COLUMN IF NOT EXISTS citizens INT NOT NULL DEFAULT 0;

-- Räkna om befintliga rader: citizens = ROUND(weight × effektivt labor_pool).
-- labor_pool approximeras som population − (infantry*5 + cavalry*8 + catapult*2 + priest*3 + ship*10 + elite_infantry*10).
UPDATE settlement_labor sl
SET citizens = GREATEST(0, ROUND(
    sl.weight * GREATEST(0,
        s.population
        - (s.infantry * 5 + s.cavalry * 8 + s.catapult * 2
           + s.priest * 3 + s.ship * 10 + s.elite_infantry * 10)
    )
))
FROM settlements s
WHERE sl.settlement_id = s.id;

-- Kräv att summan inte råkar bli noll för befintliga rader med positiv weight.
-- (Fördelning kan bli skev vid lågt population — fördela jämnt om allt landade på noll.)
UPDATE settlement_labor sl
SET citizens = 1
WHERE citizens = 0
  AND sl.weight > 0;

-- Ta bort weight-kolumnen nu när migrationen är klar.
ALTER TABLE settlement_labor DROP COLUMN IF EXISTS weight;
