DROP TABLE correspondence_citations;
CREATE OR REPLACE FUNCTION update_correspondence_letter() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE delivery_owner uuid;
BEGIN
 IF NEW.status='delivered' AND OLD.status<>'delivered' THEN
  SELECT owner_id INTO delivery_owner FROM settlements WHERE id=NEW.destination_id FOR SHARE;
 END IF;
 UPDATE correspondence_letters SET status=NEW.status, trade_offer=NEW.trade_offer,
 reply_text=NEW.reply_text,
 delivered=delivered OR COALESCE(NEW.status='delivered' AND OLD.status<>'delivered' AND recipient_id=delivery_owner,false),
 returned=returned OR NEW.status='arrived'
 WHERE id=NEW.id;
 RETURN NEW;
END $$;

ALTER TABLE correspondence_letters DROP COLUMN accepted_at,DROP COLUMN accepted_xid,DROP COLUMN accepted_epoch,DROP COLUMN returned_xid,DROP COLUMN returned_epoch;
