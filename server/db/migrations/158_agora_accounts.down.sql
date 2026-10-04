ALTER TABLE players
    DROP CONSTRAINT players_agora_account_consistent,
    DROP COLUMN agora_state,
    DROP COLUMN agora_claim_localpart,
    DROP COLUMN agora_localpart;
