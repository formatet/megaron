-- Migration 133: skeppet bär sin egen mat (megaron_plan_skeppsproviant.md §3,
-- beslutad av Timothy 2026-08-26).
--
-- Fram till nu TELEPORTERADE sjöupkeepet: internal/combat/upkeep.go läser
-- support_settlement_id utan någon distansterm, så en galär tjugo hexar ut åt
-- ur hemstadens magasin varje tick, omedelbart. Maten struntade i den regel
-- allt annat i spelet lyder (budbärare är fysiska, kommando är aldrig
-- omedelbart). Provianten dras nu ur staden VID AVFÄRD, äts under resan, och
-- resten lastas av vid hemkomst.
--
-- Samma form som units.carried_silver, som redan gör exakt detta för
-- koloniseringssilver: dras ur moderstadens kassa vid utskick, inte myntat
-- vid framkomsten.
--
-- DEFAULT 0: landenheter använder aldrig fältet (proviantering är sjö,
-- furagering är land). Redan utskickade skepp står oprovianterade efter
-- deployen och faller till hemvändningsgrenen nästa tick — avsiktligt, att
-- bakåtfylla en gissad ranson vore att uppfinna data.
ALTER TABLE units ADD COLUMN provisions DOUBLE PRECISION NOT NULL DEFAULT 0
    CHECK (provisions >= 0);
