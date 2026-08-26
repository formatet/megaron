-- Migration 134: Utfodringsordningen — dagens obetäckta matbehov
-- (megaron_plan_utfodringsordningen.md D4)
--
-- Befolkningens konsumtion vecklas ut ur grain-takten till ett eget diskret
-- steg, Föda (prioritet 55, mellan Plikt 50 och Tillväxt 60). Steget debiterar
-- dagens behov ur LAGRET i kanonordning grain → fisk → boskap
-- (economy.FoodConsumptionSplit) och skriver vad som blev KVAR obetäckt hit —
-- utfallet, aldrig avsikten (CLAUDE.md "Events").
--
-- KharisTick's tillväxt/svält-gren (internal/kharis/tick.go) och belägringens
-- svält-klocka (applySiegeStarvationClock) läser båda den här kolumnen i
-- stället för grain_now > 0 respektive grain's rate < 0 — det gamla ankaret
-- slutar fungera i och med D1 (grain-takten blir brutto och kan aldrig bli
-- negativ längre).
--
-- Backfill: DEFAULT 0 sätter kolumnen till "fullt utfodrad" för varje
-- befintlig bosättning i samma ALTER — ingen separat UPDATE behövs.
ALTER TABLE settlements ADD COLUMN food_unmet_amount DOUBLE PRECISION NOT NULL DEFAULT 0;

COMMENT ON COLUMN settlements.food_unmet_amount IS
    'Gårdagens obetäckta matbehov i matenheter (grain-ekvivalent), skrivet av FoodTick (prioritet 55). 0 = befolkningen fick allt den behövde. Se migration 134.';
