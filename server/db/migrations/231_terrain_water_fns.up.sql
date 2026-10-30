-- Water classification owner in SQL (insjöar slice 1). Mirrors
-- internal/province: IsSea / IsNavigable. Inlinable on purpose, and an IN list
-- keeps the NULL semantics of the NOT IN lists these replace.
CREATE OR REPLACE FUNCTION terrain_is_sea(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea') $$;

CREATE OR REPLACE FUNCTION terrain_is_navigable(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea','river','river_ford') $$;
