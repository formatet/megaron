-- Explore order (auto-return): the unit's settlement_id is nulled on dispatch
-- (same as any march), so the home settlement to return to must be captured
-- separately at explore-dispatch time and carried across the outbound leg.
ALTER TABLE units ADD COLUMN home_settlement_id UUID REFERENCES settlements(id) ON DELETE SET NULL;
