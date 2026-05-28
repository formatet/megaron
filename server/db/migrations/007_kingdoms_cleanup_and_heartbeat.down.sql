-- Reverse migration 007.
DROP TABLE IF EXISTS server_heartbeats;

-- Restore "basileus" → "king" (data loss for any rows that were "advisor" is acceptable in rollback).
UPDATE kingdom_members SET role = 'king' WHERE role = 'basileus';
