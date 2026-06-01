-- Add structured trade offer to messengers.
-- trade_offer JSONB: {want_good, want_qty, offer_gold, status}
-- status: 'pending' | 'accepted' | 'declined'
ALTER TABLE messengers ADD COLUMN IF NOT EXISTS trade_offer JSONB;
