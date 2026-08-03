-- Down for 106. Restores the COLUMN, not the DATA.
--
-- The up migration moved every fund balance into the settlement's liquid silver
-- and raised the cap by the same amount. Down cannot tell that silver apart from
-- silver the city earned since, so it leaves it where it is and brings the column
-- back at 0. Running down therefore does not recreate the pre-106 world; it makes
-- the schema loadable by pre-106 code, with every fund empty.
--
-- Food stored in the granary is DROPPED with the table. Move it back into the
-- cities before running this if the amounts matter.

ALTER TABLE settlements ADD COLUMN sitos_fund_silver float8 NOT NULL DEFAULT 0;

DROP TABLE settlement_granary;
