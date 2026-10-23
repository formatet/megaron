DROP INDEX idx_messengers_correspondence_page;
ALTER TABLE messengers DROP COLUMN correspondence_created_xid;
ALTER TABLE messengers DROP COLUMN correspondence_created_epoch;
