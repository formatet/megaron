-- Migration 148: budet liftar (megaron_plan_budet_liftar.md, slice 3a).
--
-- A messenger whose route needs sea now runs to its own nearest coastal/harbour
-- port and waits there (passage_status='awaiting_passage') instead of crossing
-- the abstract "boat" instantly. It boards an eligible carrier (a real
-- transport or a ship mission departing that port) when one leaves
-- (passage_status='aboard', carrier_transport_id/carrier_unit_id set), or —
-- lost with no carrier — takes the old abstract passage after a wait (the
-- RESERVE, removed in slice 3b). A captured/limped/sunk carrier returns the
-- messenger to its port, sealed, after a fixed delay (passage_status=
-- 'returning_sealed', passage_lost_until_tick).
--
-- passage_since_tick is the R5 reserve clock: the tick passage_status last
-- became 'awaiting_passage' (initial port arrival, or promotion out of
-- 'returning_sealed'). All six columns are NULL for a purely-land messenger —
-- the whole mechanic is additive and inert for the common case.
ALTER TABLE messengers
    ADD COLUMN passage_status TEXT
        CHECK (passage_status IN ('awaiting_passage', 'aboard', 'returning_sealed')),
    ADD COLUMN passage_port_id UUID REFERENCES settlements(id),
    ADD COLUMN passage_since_tick INT,
    ADD COLUMN passage_lost_until_tick INT,
    ADD COLUMN carrier_transport_id UUID REFERENCES transports(id),
    ADD COLUMN carrier_unit_id UUID REFERENCES units(id),
    ADD COLUMN carrier_name TEXT;

CREATE INDEX idx_messengers_passage_waiting ON messengers (passage_port_id)
    WHERE passage_status = 'awaiting_passage';
CREATE INDEX idx_messengers_passage_aboard ON messengers (carrier_transport_id)
    WHERE passage_status = 'aboard' AND carrier_transport_id IS NOT NULL;
