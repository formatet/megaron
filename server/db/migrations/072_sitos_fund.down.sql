-- Reverse migration 072: drop the Sitos fund column.
ALTER TABLE settlements DROP COLUMN sitos_fund_silver;
