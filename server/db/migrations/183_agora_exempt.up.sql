ALTER TABLE players ADD COLUMN agora_exempt boolean NOT NULL DEFAULT false;

-- Existing playtest accounts are already registered. Runtime eligibility reads
-- only the explicit flag; an ordinary future registration keeps the default.
UPDATE players SET agora_exempt = true
WHERE username LIKE 'luna-%' OR username LIKE 'gpt-%';

COMMENT ON COLUMN players.agora_exempt IS
    'Explicit admin exemption from Agora provisioning and password access. Set before an AI account founds a city.';
