-- Down for 109. Drops the public display name; the login (username) is
-- untouched. Every read surface that switched to COALESCE(wanax_name,
-- username) falls back to username automatically once this column is gone.

DROP INDEX IF EXISTS players_wanax_name_key;
ALTER TABLE players DROP COLUMN wanax_name;
