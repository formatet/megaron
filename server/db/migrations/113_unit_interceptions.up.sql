-- Migration 113: unit_interceptions — the double-avskärning guard for the
-- unit-vs-unit sentry scan (megaron_plan_avsiktslagret.md §S3,
-- combat/unit_intercept_scan.go). A marching unit's own status never changes
-- when it survives an interception (the march continues — full
-- march-interruption semantics are KR2's job, not this substrate), so the
-- SAME (marching unit, sentry) pair would otherwise fight again on every scan
-- tick it remains in the sentry's radius. depart_tick identifies the march
-- INSTANCE (a fresh march sets a new depart_tick), so a unit that returns
-- home and marches out again past the same sentry can be intercepted anew.
CREATE TABLE unit_interceptions (
    unit_id        UUID        NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    sentry_unit_id UUID        NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    depart_tick    INT         NOT NULL,
    world_id       UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    intercepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (unit_id, sentry_unit_id, depart_tick)
);
