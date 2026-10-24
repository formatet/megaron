-- A route is one shipment. Its complete manifest belongs to its physical mover;
-- the legacy scalar columns remain intact for pre-V2 rows and clients.
ALTER TABLE trade_routes ADD COLUMN transport_id uuid REFERENCES transports(id) ON DELETE SET NULL;
