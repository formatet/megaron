-- Kingdom forming state: kingdoms start 'forming' and become 'active' at 3+ members.
-- Benefits (shared fog, tribute) only apply when active.
ALTER TABLE kingdoms ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'forming'
    CHECK (state IN ('forming','active','dissolved'));

-- Backfill: kingdoms with ≥3 members are already active.
UPDATE kingdoms k SET state = 'active'
WHERE (SELECT count(*) FROM kingdom_members WHERE kingdom_id = k.id) >= 3;
