-- Migration 144: ge redan befintliga eskortkohorter sina regementsnummer
-- (Timothy 2026-09-25: "det behöver fixas").
--
-- Från och med granskning/paritet skriver seedNomadicHost ordinal 1 och 2 på
-- eskorten vid join, så den heter "1st/2nd Spearmen of <Wanax>" före
-- grundningen (unit.LandUnitName). Wanaxer som anslöt FÖRE det har NULL-ordinal
-- och ser "Spearmen of <Wanax>" två gånger. Den här raden ger dem 1, 2, … per
-- (värld, ägare, typ) i skapelseordning — samma ordning foundMetropolis sedan
-- behåller när staden grundas.
--
-- Bara enheter UTAN stad (support_settlement_id IS NULL) hos en Wanax som ännu
-- inte äger någon bosättning i världen: det är exakt eskorten före grundning.
-- Idempotent: rör bara NULL-ordinaler.
UPDATE units u
   SET ordinal = n.rn
  FROM (
    SELECT u2.id,
           ROW_NUMBER() OVER (PARTITION BY u2.world_id, u2.owner_id, u2.type ORDER BY u2.created_at, u2.id) AS rn
      FROM units u2
     WHERE u2.category = 'land'
       AND u2.type <> 'nomadic_host'
       AND u2.status <> 'disbanded'
       AND u2.support_settlement_id IS NULL
       AND u2.ordinal IS NULL
       AND NOT EXISTS (SELECT 1 FROM settlements s
                        WHERE s.world_id = u2.world_id AND s.owner_id = u2.owner_id)
  ) n
 WHERE u.id = n.id;
