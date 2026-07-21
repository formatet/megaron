-- Return-leg departure timestamp for a replied messenger (temenos_orderlopare_plan
-- §(b) returbens-ögat). When a recipient replies, the courier turns around and
-- runs home; the return leg needs its OWN time window so the hemerodromos eye and
-- the map renderer can interpolate its position dest→origin. sent_at must stay the
-- ORIGINAL send time (the correspondence log in ListFromHost/ListSent shows and
-- orders by it), so the return departure gets a dedicated column rather than
-- overloading sent_at. Nullable: only set on reply; NULL for outbound/never-replied.
ALTER TABLE messengers ADD COLUMN IF NOT EXISTS return_departs_at TIMESTAMPTZ;
