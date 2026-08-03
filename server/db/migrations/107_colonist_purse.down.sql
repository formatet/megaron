-- Down for 107. Any silver in transit on a unit is dropped with the column —
-- it was already debited from its mother city, so running this while an
-- expedition is on the road destroys that silver. Land your colonists first.

ALTER TABLE units DROP COLUMN carried_silver;
