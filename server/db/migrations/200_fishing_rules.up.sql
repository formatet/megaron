-- ORDER14 final rule: deep-water fish has one base unit per fisher and tick.
UPDATE production_rules SET rate_per_tick=1
 WHERE terrain_type='deep_sea' AND good_key='fish' AND building_type IS NULL;
-- Code owns context/capacity and trims + recomputes atomically before HTTP/workers.
CREATE TABLE fishing_rule_transition (
 id boolean PRIMARY KEY DEFAULT true CHECK (id),
 completed boolean NOT NULL DEFAULT false
);
INSERT INTO fishing_rule_transition(id) VALUES(true);
