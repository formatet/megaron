-- No historical recipient can be inferred from a city's present owner. Only
-- application-created new correspondence is attached; legacy rows stay legacy.
CREATE TABLE correspondence_cases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 world_id uuid NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 first_player_id uuid NOT NULL REFERENCES players(id),
 second_player_id uuid NOT NULL REFERENCES players(id),
 subject text NOT NULL CHECK(length(subject) BETWEEN 1 AND 120),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 created_epoch timestamptz NOT NULL DEFAULT pg_postmaster_start_time()
);
CREATE INDEX correspondence_cases_people ON correspondence_cases(world_id,first_player_id,second_player_id,created_at DESC,id DESC);
-- IDs/names of cities are snapshots, deliberately without settlement FKs.
-- Messenger deletion/cascade does not delete a person's private archive.
CREATE TABLE correspondence_letters (
 id uuid PRIMARY KEY,
 case_id uuid NOT NULL REFERENCES correspondence_cases(id) ON DELETE CASCADE,
 sender_id uuid NOT NULL REFERENCES players(id),
 recipient_id uuid NOT NULL REFERENCES players(id),
 sender_name text NOT NULL, recipient_name text NOT NULL,
 origin_id uuid NOT NULL, destination_id uuid NOT NULL,
 origin_name text NOT NULL, destination_name text NOT NULL,
 message_text text NOT NULL, reply_text text,
 sent_at timestamptz NOT NULL,
 status text NOT NULL,
 trade_offer jsonb,
 delivered boolean NOT NULL DEFAULT false,
 returned boolean NOT NULL DEFAULT false,
 created_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 created_epoch timestamptz NOT NULL DEFAULT pg_postmaster_start_time()
);
CREATE INDEX correspondence_letters_case ON correspondence_letters(case_id,sent_at DESC,id DESC);
-- Only physical outcomes are mutable. Identity and original text never change.
CREATE FUNCTION update_correspondence_letter() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER correspondence_letter_outcomes AFTER UPDATE OF status,trade_offer,reply_text ON messengers
 FOR EACH ROW EXECUTE FUNCTION update_correspondence_letter();
-- Legacy operational access remains unchanged; new letters require their frozen recipient.
CREATE FUNCTION correspondence_recipient(letter uuid, person uuid) RETURNS boolean
 LANGUAGE sql STABLE AS $$ SELECT NOT EXISTS(SELECT 1 FROM correspondence_letters WHERE id=letter)
 OR EXISTS(SELECT 1 FROM correspondence_letters WHERE id=letter AND recipient_id=person AND delivered) $$;
