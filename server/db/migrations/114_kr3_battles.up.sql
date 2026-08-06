-- Migration 114: KR3 stridssystemet — substratet (megaron_plan_kr3_stridssystem.md §1/§8).
--
-- Three tables replace the one-shot resolveCombat/resolveFieldCombat model with a
-- battle that is PERSISTENT and RESOLVED OVER MULTIPLE TICKS:
--
--   battles             — the durable engagement at one hex.
--   battle_participants — which units are in it, when they joined/left, their
--                         size trajectory.
--   battle_rounds       — the append-only, reproducible dice log (PK on
--                         (battle_id, tick_index, round_index) gives G2
--                         idempotency for free — a re-run handler that already
--                         wrote this round's row cannot write it twice).
--
-- This slice wires ONLY the field-arrival entry point (combat/unit_arrival_field.go).
-- The other three initiation points named in the plan (settlement resolveCombat,
-- amphibious assault, avsiktslagret's unit_intercept_scan.go) still use the old
-- one-shot resolve — they are rewired in a later slice, not here.

CREATE TABLE battles (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id           uuid        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    q                  int         NOT NULL,
    r                  int         NOT NULL,
    started_tick       int         NOT NULL,
    current_tick       int         NOT NULL,
    status             text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'ended')),
    termination_reason text        CHECK (termination_reason IN
                            ('annihilation', 'rout', 'retreat_order', 'attacker_reached_city', 'no_enemy_left')),
    -- Seed is drawn once at initiation from economy.Dice (never time.Now()/global
    -- rand — CLAUDE.md time rule). Every round's dice stream is re-derived
    -- deterministically from (seed, tick_index, round_index), so any round can be
    -- replayed exactly from just this column plus its battle_rounds row.
    seed               bigint      NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

-- At most one ACTIVE battle per hex per world — this is what makes
-- initiateOrJoinBattle's "does an active battle already sit here" check a
-- simple, race-safe lookup (SELECT ... FOR UPDATE against this index) instead
-- of an application-level race between two arrivals on the same tick.
CREATE UNIQUE INDEX battles_active_hex_uniq ON battles (world_id, q, r) WHERE status = 'active';

CREATE INDEX battles_world_status_idx ON battles (world_id, status);

CREATE TABLE battle_participants (
    battle_id          uuid        NOT NULL REFERENCES battles(id) ON DELETE CASCADE,
    unit_id            uuid        NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    owner_id           uuid        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    side               text        NOT NULL CHECK (side IN ('attacker', 'defender')),
    joined_tick        int         NOT NULL,
    left_tick          int,
    initial_size       int         NOT NULL,
    current_size       int         NOT NULL,
    -- Reserved for §5/§7 (stood reträttorder, avsiktslagret reaction_policy verbs
    -- escort/alert) — not read or written by this slice's handler, just shaped so
    -- a later slice can populate it without a schema change.
    standing_orders    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- The participation fraction (§4) sampled at this unit's most recent
    -- battle-tick — nullable because a just-joined participant hasn't been
    -- sampled yet this tick.
    last_participation float8,
    PRIMARY KEY (battle_id, unit_id)
);

CREATE INDEX battle_participants_battle_idx ON battle_participants (battle_id) WHERE left_tick IS NULL;

CREATE TABLE battle_rounds (
    battle_id      uuid        NOT NULL REFERENCES battles(id) ON DELETE CASCADE,
    tick_index     int         NOT NULL,
    round_index    int         NOT NULL,
    -- Per side: {"active_combatants": n, "dice_rolled": n, "hits_caused": n, "losses_received": n}
    attacker       jsonb       NOT NULL,
    defender       jsonb       NOT NULL,
    rout_checks    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    orders_applied jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (battle_id, tick_index, round_index)
);
