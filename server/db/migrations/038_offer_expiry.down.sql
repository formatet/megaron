DROP INDEX IF EXISTS idx_messengers_offer_expiry;
ALTER TABLE messengers DROP COLUMN IF EXISTS expires_at;
