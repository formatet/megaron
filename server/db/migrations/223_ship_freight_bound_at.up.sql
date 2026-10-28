-- A ship leaves garrison for freight only through BindShip. From that moment the
-- unit row (where it docked) is the location witness; only transports created
-- after the binding move it. NULL keeps legacy bound ships on their leg history.
ALTER TABLE units ADD COLUMN freight_bound_at timestamptz;
