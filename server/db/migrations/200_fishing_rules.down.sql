DROP TABLE fishing_rule_transition;
UPDATE production_rules SET rate_per_tick=1.0/6
 WHERE terrain_type='deep_sea' AND good_key='fish' AND building_type IS NULL;
