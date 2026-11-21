UPDATE units SET origin_settlement_id = support_settlement_id WHERE origin_settlement_id IS NULL AND support_settlement_id IS NOT NULL AND status <> 'disbanded';
