-- Migration 116: KR3 mur-modellen (megaron_kr3_stridsutvardering.md beslut 7,
-- megaron_plan_kr3_stridssystem.md §8). The wall is a SHIELD, not a strength
-- multiplier: each battle-tick it absorbs the first N incoming hits on the
-- DEFENDING side, before losses are applied. N scales with wall level and
-- must be snapshotted once at battle start (alongside seed) so a besieged
-- settlement's wall level cannot drift mid-battle out from under an
-- in-progress fight.
--
-- storm is likewise snapshotted once at battle start — whether the attacking
-- participant held storm stance when the battle began. Storm halves the
-- wall's absorption but raises the storming attacker's own losses
-- (combat/battle.go). Field battles and interception have no settlement at
-- their hex, so wall_level is always 0 there — the absorption term is a
-- no-op for both, unchanged from before this migration.
ALTER TABLE battles
    ADD COLUMN wall_level int  NOT NULL DEFAULT 0,
    ADD COLUMN storm      bool NOT NULL DEFAULT false;
