-- Namnstandardens bärande kolumn + regementsordinalen (megaron_aktorer_plan.md
-- Fas 3 / §3.1, avgjort av Timothy 2026-07-26).
--
-- Före detta fanns ingen stabil sanning om VILKEN stad som betalar ett förband:
-- upkeep.go:s payingSettlement() returnerade units.settlement_id om den var satt
-- och annars ÄGARENS HUVUDSTAD som tyst fallback. Ett skepp till sjöss och varje
-- marscherande enhet betalades alltså av huvudstaden av misstag, inte av design.
--
-- units.home_settlement_id ser ut att lösa detta men gör motsatsen: den fångas
-- vid utmarsch och sätts till NULL vid hemkomst (combat/unit_arrival.go). Den är
-- transient marschorigo och rörs inte här.
--
-- support_settlement_id är PERMANENT: den sätts vid rekrytering och ändras
-- aldrig av marsch, hemkomst eller omstationering. Ingen skrivande endpoint får
-- finnas — spelaren kan inte flytta ett förbands försörjande stad. Faller staden
-- betalas ingen sold, och förbandet deserterar via befintlig unpaid_periods +
-- applyAttrition. Den tystnaden är just dagens bugg och ska inte återinföras.
--
-- ON DELETE SET NULL, inte RESTRICT: en förstörd stad SKA lämna sina förband
-- obetalda. Det är regeln, inte ett fel som ska blockeras.
ALTER TABLE units ADD COLUMN support_settlement_id UUID
    REFERENCES settlements(id) ON DELETE SET NULL;

CREATE INDEX idx_units_support_settlement ON units(support_settlement_id)
    WHERE support_settlement_id IS NOT NULL;

-- Regementsnumret. Räknas per (stad, enhetstyp).
ALTER TABLE units ADD COLUMN ordinal INT;

-- Monoton räknare. **Ordinalen återanvänds ALDRIG** (Timothy 2026-07-26:
-- "historiska regementen får isf vara historiska"). Upplöses 2nd Spearmen of
-- Knossos blir nästa rekryt 4th, inte 2nd — numret är organisatorisk historia,
-- inte en ledig plats. Därför en egen räknare i stället för MAX(ordinal)+1, som
-- skulle återanvända numret så fort ett förband försvann.
CREATE TABLE unit_ordinals (
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    unit_type     TEXT NOT NULL,
    next_ordinal  INT  NOT NULL DEFAULT 1,
    PRIMARY KEY (settlement_id, unit_type)
);

-- ── Backfill ──────────────────────────────────────────────────────────────
-- Engångsåtgärd. Efter detta finns ingen fallback i kodvägen: en enhet utan
-- support_settlement_id behandlas som obetald, punkt.
UPDATE units u
   SET support_settlement_id = COALESCE(
        u.settlement_id,
        (SELECT s.id FROM settlements s
          WHERE s.owner_id = u.owner_id AND s.world_id = u.world_id
            AND s.is_capital = true
          LIMIT 1))
 WHERE u.status IN ('garrison', 'marching', 'positioned', 'forming', 'embarked');

-- Ordinalen tilldelas i created_at-ordning per (stad, typ), så befintliga
-- förband får numren i den ordning de faktiskt restes.
WITH numbered AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY support_settlement_id, type
                              ORDER BY created_at, id) AS n
      FROM units
     WHERE support_settlement_id IS NOT NULL
)
UPDATE units u SET ordinal = numbered.n
  FROM numbered WHERE numbered.id = u.id;

-- Räknaren startar efter det högsta utdelade numret — inklusive numren på redan
-- upplösta förband, annars vore den första nyrekryteringen efter migrationen en
-- återanvändning.
INSERT INTO unit_ordinals (settlement_id, unit_type, next_ordinal)
SELECT support_settlement_id, type, MAX(ordinal) + 1
  FROM units
 WHERE support_settlement_id IS NOT NULL AND ordinal IS NOT NULL
 GROUP BY support_settlement_id, type;
