-- Migration 038: handelsoffert-utgång (TTL)
--
-- Lägger expires_at på messengers-tabellen. En trade-offer-messenger
-- med expires_at < now() döljs i inboxen och behandlas som resolved.
-- Befintliga rader: NULL = löper ut aldrig (bakåtkompatibelt).
-- Nya trade-offer-messengers sätter expires_at = arrives_at + 7 dagar
-- (appliceras i applikationslagret; migration seedar inte existerande).

ALTER TABLE messengers ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Index för inbox-queryn (filtrerar ut utgångna erbjudanden effektivt).
CREATE INDEX IF NOT EXISTS idx_messengers_offer_expiry
    ON messengers (world_id, expires_at)
    WHERE trade_offer IS NOT NULL;
