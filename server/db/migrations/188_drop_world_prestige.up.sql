-- worlds.prestige was accumulated by the kharis tick but no rule, web or keryx
-- surface ever read it. Kingdom prestige (kingdoms.prestige) is a separate column.
ALTER TABLE worlds DROP COLUMN prestige;
