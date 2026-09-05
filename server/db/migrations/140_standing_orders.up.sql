-- Migration 140: standing orders (megaron_plan_staende_leverans.md) — caravans
-- that run themselves between two of a Wanax's own settlements.
--
-- PULL, not push (plan §1): a standing order names a destination good/threshold
-- ("keep grain at Colony ≥ 200"), not a fixed quantity/interval. The recurring
-- sweep (internal/combat, ScheduledStandingOrderTick) reads the live shortfall
-- each tick and reuses the existing internal-transfer rail (transport.Dispatch,
-- transport.Manifest, transport.ArrivalHandler) — no new transport mechanic.
--
-- crewed_by_settlement_id is which end supplies the gubbe (plan §4c: usually the
-- grain-source end, but the schema doesn't hardcode which — CHECK only requires
-- it be one of the two route ends). from/to are directional for the OUTBOUND
-- leg; the return leg (plan §4c, "a route is a round trip") runs to→from
-- automatically once the outbound caravan lands, carrying whatever the return
-- goods list says to bring home, floored so the destination never gets stripped.
CREATE TABLE standing_orders (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id                UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id                UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    from_settlement_id      UUID        NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    to_settlement_id        UUID        NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    crewed_by_settlement_id UUID        NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    status                  TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    -- pause_reason is player-facing text set by the sweep itself (trap §2/§3 of
    -- the plan: insufficient surplus at the source, or no spare workforce at the
    -- crewing settlement) — never silent, per the plan's worst-case warning.
    pause_reason            TEXT,
    last_dispatched_tick    INT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_settlement_id <> to_settlement_id),
    CHECK (crewed_by_settlement_id = from_settlement_id OR crewed_by_settlement_id = to_settlement_id)
);

CREATE INDEX idx_standing_orders_world_active ON standing_orders (world_id) WHERE status = 'active';

-- Outbound list: goods to keep topped up AT THE DESTINATION, pulled from the
-- source. threshold is the floor to maintain at to_settlement_id (plan §2:
-- "keep grain at Colony ≥ 200").
CREATE TABLE standing_order_outbound_goods (
    standing_order_id UUID    NOT NULL REFERENCES standing_orders(id) ON DELETE CASCADE,
    good_key          TEXT    NOT NULL REFERENCES goods(key),
    threshold         FLOAT   NOT NULL CHECK (threshold >= 0),
    PRIMARY KEY (standing_order_id, good_key)
);

-- Return list: goods to bring HOME from the destination, leaving at least
-- `floor` behind (plan §4c: the poor end never pays a gubbe, it only loads the
-- caravan that is already standing in its port).
CREATE TABLE standing_order_return_goods (
    standing_order_id UUID    NOT NULL REFERENCES standing_orders(id) ON DELETE CASCADE,
    good_key          TEXT    NOT NULL REFERENCES goods(key),
    floor             FLOAT   NOT NULL DEFAULT 0 CHECK (floor >= 0),
    PRIMARY KEY (standing_order_id, good_key)
);

-- Ties a physical transports row to the standing order that dispatched it —
-- the substrate for both traps the plan calls out: trap 1 (§2, "must count
-- caravans already in flight") is a query for an in_transit row with this
-- column set; trap 2 (§3, the source's own need) is evaluated at dispatch
-- time and never needs to look at this column at all. ON DELETE SET NULL: a
-- caravan already under way keeps flying even if the order that sent it is
-- deleted afterward — the row becomes an ordinary untagged transfer.
ALTER TABLE transports ADD COLUMN standing_order_id UUID REFERENCES standing_orders(id) ON DELETE SET NULL;

CREATE INDEX idx_transports_standing_order ON transports (standing_order_id) WHERE standing_order_id IS NOT NULL;
