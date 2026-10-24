-- Persist when an acceptance became observable in this archive, independently
-- of mutable list order. Existing accepted/completed rows prove acceptance but not its
-- original timestamp: accepted_at deliberately stays NULL during backfill.
ALTER TABLE correspondence_letters ADD COLUMN accepted_at timestamptz;
ALTER TABLE correspondence_letters ADD COLUMN accepted_xid xid8;
ALTER TABLE correspondence_letters ADD COLUMN accepted_epoch timestamptz;
ALTER TABLE correspondence_letters ADD COLUMN returned_xid xid8;
ALTER TABLE correspondence_letters ADD COLUMN returned_epoch timestamptz;
UPDATE correspondence_letters SET accepted_xid=pg_current_xact_id(),accepted_epoch=pg_postmaster_start_time()
 WHERE trade_offer->>'status' IN ('accepted','returned');
UPDATE correspondence_letters SET returned_xid=pg_current_xact_id(),returned_epoch=pg_postmaster_start_time() WHERE returned;

CREATE OR REPLACE FUNCTION update_correspondence_letter() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE delivery_owner uuid;
BEGIN
 IF NEW.status='delivered' AND OLD.status<>'delivered' THEN
  SELECT owner_id INTO delivery_owner FROM settlements WHERE id=NEW.destination_id FOR SHARE;
 END IF;
 UPDATE correspondence_letters SET status=NEW.status, trade_offer=NEW.trade_offer,
 reply_text=NEW.reply_text,
 delivered=delivered OR COALESCE(NEW.status='delivered' AND OLD.status<>'delivered' AND recipient_id=delivery_owner,false),
 returned=returned OR NEW.status='arrived',
 accepted_at=CASE WHEN accepted_xid IS NULL AND NEW.trade_offer->>'status'='accepted' THEN clock_timestamp() ELSE accepted_at END,
 accepted_xid=CASE WHEN accepted_xid IS NULL AND NEW.trade_offer->>'status'='accepted' THEN pg_current_xact_id() ELSE accepted_xid END,
 accepted_epoch=CASE WHEN accepted_xid IS NULL AND NEW.trade_offer->>'status'='accepted' THEN pg_postmaster_start_time() ELSE accepted_epoch END,
 returned_xid=CASE WHEN returned_xid IS NULL AND NEW.status='arrived' THEN pg_current_xact_id() ELSE returned_xid END,
 returned_epoch=CASE WHEN returned_xid IS NULL AND NEW.status='arrived' THEN pg_postmaster_start_time() ELSE returned_epoch END
 WHERE id=NEW.id;
 RETURN NEW;
END $$;

CREATE TABLE correspondence_citations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 case_id uuid NOT NULL REFERENCES correspondence_cases(id) ON DELETE CASCADE,
 player_id uuid NOT NULL REFERENCES players(id),
 letter_id uuid NOT NULL REFERENCES correspondence_letters(id) ON DELETE CASCADE,
 source text NOT NULL CHECK(source IN ('message','reply')),
 quote text NOT NULL CHECK(octet_length(quote) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 created_epoch timestamptz NOT NULL DEFAULT pg_postmaster_start_time(),
 UNIQUE(player_id,letter_id,source,quote)
);
CREATE INDEX correspondence_citations_person ON correspondence_citations(player_id,case_id,created_at DESC,id DESC);
