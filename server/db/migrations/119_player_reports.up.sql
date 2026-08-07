-- Migration 119: player_reports (B1, megaron_mvp_mandag.md §B1).
--
-- The MVP had no way at all for a testing Wanax to tell Timothy something was
-- broken or felt wrong — zero grep hits on bugreport/feedback/report_issue
-- before this. Deliberately primitive: one table, one POST, one admin-gated
-- GET. q/r/view are nullable because a report isn't always tied to a place on
-- the map (e.g. "the notification text is unreadable"); tick is stamped by
-- the server from current_world_tick(), never typed by the player — the
-- point of this table is that the expensive context (who, when, where) is
-- free, and the player only supplies what a machine can't infer.
CREATE TABLE player_reports (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id   UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('bug', 'design', 'confused')),
    body       TEXT NOT NULL,
    q          INT,
    r          INT,
    view       TEXT,
    tick       INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON player_reports(world_id, created_at DESC);
