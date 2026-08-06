-- Migration 109: wanax_name — a public display name separate from the login.
--
-- Timothy 2026-08-05: "vi måste börja dela ut minoiska namn random på spelare
-- så att folk inte använder deras logins". Until now players.username was
-- BOTH the login and the only name every other Wanax ever saw — a settlement
-- marker, a foreign-units row, a kingdom roster all leaked the login string.
--
-- wanax_name is the new public identity. Existing players keep their current
-- name — a backfill to their own username, so nobody's identity changes mid-
-- world — and new joiners get a fresh minoan name assigned by
-- province.UniqueWanaxName at join (server/internal/province/wanaxnames.go).
--
-- Unique for the same reason a settlement name is unique: it is the address
-- other Wanax use to recognise you, and two people answering to the same name
-- would be confusing in exactly the way two cities sharing a name would be.
-- Partial (WHERE wanax_name IS NOT NULL) rather than a plain UNIQUE column
-- constraint, since the column itself is nullable during rollout.

ALTER TABLE players ADD COLUMN wanax_name TEXT;

UPDATE players SET wanax_name = username WHERE wanax_name IS NULL;

CREATE UNIQUE INDEX players_wanax_name_key ON players (wanax_name) WHERE wanax_name IS NOT NULL;

COMMENT ON COLUMN players.wanax_name IS
    'Public display name shown to other Wanax — separate from username (the '
    'login). NULL until join assigns one (province.UniqueWanaxName); existing '
    'rows are backfilled to their username at migration time so nobody loses '
    'their identity mid-world.';
