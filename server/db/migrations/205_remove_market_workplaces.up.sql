-- Timothy 2026-10-22: markets do not employ citizens. Retain the old rules
-- only for an exact schema rollback; never restore someone's old assignment.
CREATE TABLE retired_market_rules_205 AS SELECT * FROM production_rules WHERE building_type='market';
DELETE FROM settlement_placement WHERE target_kind='building' AND building_type='market';
DELETE FROM production_rules WHERE building_type='market';
