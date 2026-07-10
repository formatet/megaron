-- Migration 080: physical goods transports (movement-motor transport layer, Del 3-fas-1)
--
-- Until now goods in transit (internal transfers, trade deliveries) were pure
-- scheduled_events: a scalar delay + destination, with no position on the map.
-- Nothing could see them, and nothing could intercept them. This table makes a
-- goods shipment a PHYSICAL mover, modelled on `messengers`: it has an origin and
-- a destination hex, a departure/arrival time, and its live position is computed
-- lazily by re-walking its FindPath route (province.InterpolatePosition) — the same
-- lazy-position pattern as marching units and messengers.
--
-- The manifest (which goods, how much) lives in transport_goods so one caravan can
-- carry several goods. `interceptable` distinguishes a trade/transfer caravan (true)
-- from a sacred messenger (never modelled here — messengers stay uninterceptable).
-- Arrival is still tick-driven (ScheduledTransportArrival on due_tick); interception
-- (Del 3-fas-4) mutates status to 'intercepted' so the arrival handler skips delivery.

CREATE TABLE transports (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id      UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id      UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    kind          TEXT        NOT NULL,          -- 'transfer' (own→own) | 'trade' | 'trade_return'
    origin_id     UUID        REFERENCES settlements(id) ON DELETE SET NULL,
    dest_id       UUID        REFERENCES settlements(id) ON DELETE SET NULL,

    -- Routing: 'land' = caravan, 'naval' = ship (passed to province.FindPath).
    category      TEXT        NOT NULL DEFAULT 'land',
    origin_q      INT         NOT NULL,
    origin_r      INT         NOT NULL,
    dest_q        INT         NOT NULL,
    dest_r        INT         NOT NULL,

    departs_at    TIMESTAMPTZ NOT NULL,
    arrives_at    TIMESTAMPTZ NOT NULL,
    due_tick      INT         NOT NULL,

    status        TEXT        NOT NULL DEFAULT 'in_transit',  -- in_transit | delivered | intercepted | lost
    interceptable BOOLEAN     NOT NULL DEFAULT true,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Worker scan (arrival) and world-wide "what is moving" queries only ever care
-- about in-transit rows.
CREATE INDEX idx_transports_pending ON transports (due_tick) WHERE status = 'in_transit';
CREATE INDEX idx_transports_world_active ON transports (world_id) WHERE status = 'in_transit';

CREATE TABLE transport_goods (
    transport_id UUID  NOT NULL REFERENCES transports(id) ON DELETE CASCADE,
    good_key     TEXT  NOT NULL REFERENCES goods(key),
    quantity     FLOAT NOT NULL,
    PRIMARY KEY (transport_id, good_key)
);
