-- Explicit choice is presentation/protocol metadata; empty keeps legacy automatic routing.
ALTER TABLE standing_orders ADD COLUMN route_mode text NOT NULL DEFAULT '' CHECK(route_mode IN ('','land','sea'));
ALTER TABLE transports ADD COLUMN route_mode text NOT NULL DEFAULT '' CHECK(route_mode IN ('','land','sea'));
ALTER TABLE trade_routes ADD COLUMN route_mode text NOT NULL DEFAULT '' CHECK(route_mode IN ('','land','sea'));
