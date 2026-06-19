-- Migration 059: colonize-intent på diskreta enheter.
-- En marscherande enhet kan bära intent 'colonize' + ett valfritt koloninamn,
-- så ankomsthandlern (unit_arrival.go) vet att grunda en koloni i stället för
-- att bli garnison. NULL march_intent = vanlig marsch (oförändrat beteende).
-- Additiv + nullbar: säker mot den pågående skarpa körningen.
ALTER TABLE units ADD COLUMN IF NOT EXISTS march_intent TEXT;
ALTER TABLE units ADD COLUMN IF NOT EXISTS colony_name  TEXT;
