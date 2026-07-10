-- Revert migration 080: drop the physical transport tables.

DROP TABLE IF EXISTS transport_goods;
DROP TABLE IF EXISTS transports;
