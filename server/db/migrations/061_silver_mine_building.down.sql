-- Migration 061 down: återställ silver till "mine"-gating.
UPDATE production_rules SET building_type = 'mine' WHERE good_key = 'silver';
