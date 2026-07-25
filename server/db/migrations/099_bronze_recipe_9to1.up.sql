-- Migration 099: Bronze recipe copper 2 -> 9 (tin unchanged at 1)
-- Decided by Timothy 2026-07-25 (fixrunda, P2): copper is cap-pegged at 1M with no
-- sink across 10/19 cities while tin is the scarce good (220k) — 9:1 drains the
-- copper glut and yields more bronze per tin, relieving the bronze wall from the
-- copper side instead of diluting tin scarcity. Exchange rate change only, not a
-- design change ("bronsväggen ÄR spelet" stands).

UPDATE recipe_ingredients
SET quantity = 9.0
WHERE recipe_id = (
    SELECT id FROM recipes WHERE output_key = 'bronze' AND building_type = 'foundry'
)
AND good_key = 'copper';
