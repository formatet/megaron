-- Freeze actual paid costs; NULL deliberately means unknown legacy reservation.
ALTER TABLE units ADD COLUMN recruitment_costs JSONB;
ALTER TABLE units ADD COLUMN recruitment_kharis DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE units ADD COLUMN recruitment_cancelled_at TIMESTAMPTZ;
