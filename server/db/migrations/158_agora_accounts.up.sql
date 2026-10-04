ALTER TABLE players
    ADD COLUMN agora_localpart TEXT UNIQUE,
    ADD COLUMN agora_claim_localpart TEXT UNIQUE,
    ADD COLUMN agora_state TEXT,
    ADD CONSTRAINT players_agora_account_consistent CHECK ((
        (agora_state IS NULL AND agora_localpart IS NULL AND agora_claim_localpart IS NULL)
        OR
        (agora_state = 'creating' AND agora_localpart IS NULL AND agora_claim_localpart IS NOT NULL)
        OR
        (agora_state = 'ready' AND agora_localpart IS NOT NULL AND agora_localpart = agora_claim_localpart)
    ) IS TRUE);

COMMENT ON COLUMN players.agora_claim_localpart IS
    'Durable name reservation before external CREATE; retained after readiness. No password is stored.';
COMMENT ON COLUMN players.agora_localpart IS
    'Permanent ready Agora identity, attached to the game account across worlds.';
