-- Migration 095: lyft koloniernas kvarglömda lagertak
--
-- Cap-lossningen 2026-07-05 (fc8d424, "decouple pricing reference from flat
-- storage caps") gjorde lagertaket till ett icke-bindande tekniskt tak
-- (economy.goodCap = 1 000 000) och kopplade loss prissättningen från det.
-- Sveipet nådde huvudstäderna (create_metropolis.go) men missade EN seed-site:
-- foundColony i internal/combat/unit_arrival.go, som fortsatte hårdkoda de
-- gamla per-vara-taken. Följden mätt i drift 2026-07-23: 22 av 39 städer hade
-- cedar/timber-tak 500, malm 300 och craft-varor 200 — medan 17 hade 1 000 000.
--
-- Konkret skada: kolonin Handel-Och byggde ett lumbermill på skogshex, fick
-- rate 61,3 cedar/tick och ett kärl som rymde 500. Den stod vid taket och
-- brände hela sin produktion. Samma mönster hotade varje koloni som blev bra
-- på något.
--
-- Koden är fixad; den här migrationen rättar de rader som redan finns.
-- Silver rörs INTE: dess tak sätts pop-skalat av Sitos-seeden
-- (GenesisSilverLiquid) och är en riktig mekanik, inte ett kvarglömt tak.

UPDATE settlement_goods
   SET cap = 1000000
 WHERE good_key <> 'silver'
   AND cap < 1000000;
