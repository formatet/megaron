-- Migration 051: W3 catchment — ny produktionsprincip.
--
-- Stad producerar nu BARA från sin catchment (de 6 grannrutorna), inte från
-- sin egen hexruta. RecomputeProduction läser map_tiles för grannarna i stället
-- för provinces-terrängen för den egna hexan.
--
-- Befintliga settlement_goods-rater nollas (ackumulerade mängder bevaras via
-- settled()). Rater beräknas om nästa gång RecomputeProduction anropas per stad.
-- settlement_labor rensas så att auto-seedning väljer rätt varor från catchment.

UPDATE settlement_goods
SET amount  = LEAST(cap, settled(amount, rate, calc_at)),
    rate    = 0,
    calc_at = now();

DELETE FROM settlement_labor;
