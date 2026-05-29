-- Migration 006: kingdom elections + votes
-- Elections can only be called on a Sunday and lock the kingdom for 7 days.

CREATE TABLE kingdom_elections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kingdom_id      UUID NOT NULL REFERENCES kingdoms(id) ON DELETE CASCADE,
    world_id        UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    candidate_id    UUID NOT NULL REFERENCES players(id),
    called_by       UUID NOT NULL REFERENCES players(id),
    called_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    closes_at       TIMESTAMPTZ NOT NULL,
    resolved_at     TIMESTAMPTZ,
    winner_id       UUID REFERENCES players(id),
    divine_override BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX ON kingdom_elections (kingdom_id, resolved_at);
CREATE INDEX ON kingdom_elections (world_id, closes_at);

CREATE TABLE kingdom_votes (
    election_id UUID NOT NULL REFERENCES kingdom_elections(id) ON DELETE CASCADE,
    voter_id    UUID NOT NULL REFERENCES players(id),
    candidate_id UUID NOT NULL REFERENCES players(id),
    voted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (election_id, voter_id)
);

-- king_locked_until may already exist from 005; safe to skip if so.
ALTER TABLE kingdoms ADD COLUMN IF NOT EXISTS king_locked_until TIMESTAMPTZ;
