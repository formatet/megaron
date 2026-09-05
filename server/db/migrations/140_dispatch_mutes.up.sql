-- Per-player "which notification kinds become a dispatch" preference
-- (megaron_plan_dispatches.md §2/§6:4). Granularity is exact event type, not a
-- family. Absence of a row means enabled — a new notification kind is a
-- dispatch from the day it ships, with no migration and no seeding required.
-- Not world-scoped: players is a global account, and the preference is about
-- the KIND of event, the same across every world the player is in.
CREATE TABLE dispatch_mutes (
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, kind)
);
