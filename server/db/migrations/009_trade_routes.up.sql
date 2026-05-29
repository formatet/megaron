-- Migration 009: Trade routes — shipments of goods between settlements

CREATE TABLE trade_routes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id       UUID NOT NULL REFERENCES worlds(id),
    origin_id      UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    destination_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key       TEXT NOT NULL REFERENCES goods(key),
    quantity       FLOAT NOT NULL,
    departs_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    arrives_at     TIMESTAMPTZ NOT NULL,
    resolved       BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_trade_routes_origin      ON trade_routes (origin_id);
CREATE INDEX idx_trade_routes_destination ON trade_routes (destination_id);
