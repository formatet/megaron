-- Migration 103: vin odlas i hela Medelhavet, inte bara på hills
-- (Timothy 2026-07-28: "vi får nog acceptera att det går att odla druvor
-- överallt", planerarens kalibrering "hills behåller övertaget" — godkänd.)
--
-- PROBLEM: vin producerades av exakt tre production_rules-rader, alla på
-- terrain_type='hills' (migration 008 + 019). En stad utan en enda hills-hex
-- i sina sju catchment-hexar kunde alltså aldrig producera vin — och tempel
-- kräver vin varje offer (kharis.OfferWinePerTemple, tick.go:119, ur STADENS
-- EGET lager). Det låser kult för varje inlandsstad utan kulle i räckhåll,
-- och gör winery till en död byggnad utanför hills.
--
-- FIX: samma tre rader (terräng-baslinje / +farm / +winery) läggs till på
-- plains (halva hills rate — den bördiga slätten bär druvor sämre än
-- sluttningen) och på scrub_maquis (marginell mark, ingen farm-rad — man
-- odlar inte åker i snåren). Hills rörs INTE — hela poängen är att hills
-- behåller övertaget:
--
--   hills          1.2 baslinje · 2.4 med farm · 3.0 med winery   (OFÖRÄNDRAT)
--   plains         0.6 baslinje · 1.2 med farm · 1.8 med winery   (nytt, halva hills)
--   scrub_maquis   0.4 baslinje ·  (ingen farm) · 1.0 med winery  (nytt, marginellt)
--
-- Inget vin på hav, floder, deltan, berg, halvöken, stränder eller de två
-- skogarna — vinstockar vill ha jord och sol, inte kalksten eller salt.
--
-- requires_deposit är NULL på alla fem raderna med avsikt: det här är inget
-- deposit-gatat mineral, det är en gröda som växer där jorden tillåter det.
--
-- NO BACKFILL (samma resonemang som migration 092/102): kharis/tick.go och
-- alla andra 11 call sites till economy.RecomputeProduction läser
-- production_rules levande varje gång de körs. Befintliga städer plockar upp
-- de nya raderna inom en tick av nästa recompute — ingen backfill-loop behövs
-- eller är korrekt (staden kan ha bytt catchment sedan seed).
--
-- SCOPE: den här migrationen rör bara vin. Ingen annan vara (olja, oliv,
-- ceder, timmer, sten) och inte goods.base_value rebalanseras här. river_valley
-- byggs INTE — Timothy har inte uttalat sig om flodmark som vinmark; se
-- slutrapporten.

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('plains',       NULL,     'wine', 0.6, NULL),
    ('plains',       'farm',   'wine', 1.2, NULL),
    ('plains',       'winery', 'wine', 1.8, NULL),
    ('scrub_maquis', NULL,     'wine', 0.4, NULL),
    ('scrub_maquis', 'winery', 'wine', 1.0, NULL);
