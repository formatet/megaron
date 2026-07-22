-- 094: Kult är ingen vara.
--
-- Kult modellerades som en producerad vara med weight 0 — en fejk-good som
-- ingen kunde handla, äta eller bära, och vars nollvikt sprängde plundringen
-- med en division med noll (3670805). Den fanns bara för att kharis-ticken
-- summerade ett LAGER för att härleda en RATE. Det ledet stryks: ticken läser
-- tempel-tillståndet direkt.
--
-- Vad som händer med de tre spåren:
--   production_rules — bort. Ingen stad producerar kult längre.
--   settlement_goods — bort. Inget kult-lager finns kvar att summera.
--   settlement_labor — BEHÅLLS. Raden är inte längre "arbetare som gör en vara"
--                      utan HÄNGIVENHET: den andel av befolkningen som tjänar
--                      templet. Kharis-ticken läser vikten; ingenting produceras.
--                      (Vägval A, Timothy 2026-07-22/23.)
--
-- goods-raden 'cult' behålls också, men enbart som ankare för den FK
-- settlement_labor.good_key har mot goods(key). Den är inte en vara: den kan
-- inte produceras, lagras eller handlas efter denna migration.

DELETE FROM production_rules WHERE good_key = 'cult';
DELETE FROM settlement_goods WHERE good_key = 'cult';

COMMENT ON COLUMN settlement_labor.good_key IS
  'Producible good — EXCEPT ''cult'', which is devotion: the share of population serving the temple. Read by the kharis tick, produces nothing (mig 094).';
