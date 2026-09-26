-- Migration 146: sjöhandel kräver skepp (megaron_plan_sjohandel_kraver_skepp.md).
--
-- Until now `ResolveTradeRoute` could hand back "naval" for a trade/transfer/standing
-- order leg without any real vessel ever being involved — the sea route was priced and
-- timed, but no `galley`/`merchantman` was ever taken out of service to sail it. These
-- two columns let a naval transport (single-shot transfer, standing-order leg, or the
-- new hemresa/kapnings-legs this slice adds) point at the exact unit it bound.
--
-- No CHECK constraint on units.status (see mig 047's own comment on that column) — the
-- new 'freighting' value follows the same convention 'repairing' already set: gated by
-- application code (unit.go's deployable check, upkeep.go's status IN (...) list), not
-- the schema. Documented in internal/unit/model.go's Status enum, not here.
ALTER TABLE transports ADD COLUMN ship_unit_id UUID REFERENCES units(id) ON DELETE SET NULL;
ALTER TABLE standing_orders ADD COLUMN ship_unit_id UUID REFERENCES units(id) ON DELETE SET NULL;

CREATE INDEX idx_transports_ship_unit ON transports (ship_unit_id) WHERE ship_unit_id IS NOT NULL;
CREATE INDEX idx_standing_orders_ship_unit ON standing_orders (ship_unit_id) WHERE ship_unit_id IS NOT NULL;
