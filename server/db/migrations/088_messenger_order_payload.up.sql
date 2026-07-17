-- Orderkuvertet (temenos_orderlopare_plan.md Fas 2): en kurir med kind='order'
-- bär en enhetsorder som exekveras först vid leverans. Kuvertet är JSONB —
-- verb + parametrar — så nya verb (Fas 3) inte kräver schemaändring.
ALTER TABLE messengers ADD COLUMN order_payload JSONB;
