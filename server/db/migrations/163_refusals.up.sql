-- Avslagsloggen (megaron_plan_avslagslogg.md) — TILLFÄLLIG, för felsökning under
-- alfatestet, ingen spelfunktion (Timothy 2026-10-08). Varje 4xx-avslag (utom 401)
-- på ett spelarverb: vem, vilken route (chi-mönster), vilken kod, vilket tick.
-- Request-bodyn sparas aldrig. Rivs efter playtestet om den inte behövs (down = DROP).
CREATE TABLE refusals (
    id         BIGSERIAL   PRIMARY KEY,
    world_id   UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id  UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    route      TEXT        NOT NULL,
    method     TEXT        NOT NULL,
    status     INT         NOT NULL,
    code       TEXT        NOT NULL,
    world_tick INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refusals_world_player ON refusals (world_id, player_id);
