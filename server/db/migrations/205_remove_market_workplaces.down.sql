-- Workers remain idle (or wherever their owner placed them since retirement).
INSERT INTO production_rules SELECT * FROM retired_market_rules_205;
DROP TABLE retired_market_rules_205;
