-- Migration 044: Pelasger → Minoan culture rename
-- Pelasger had no distinct mechanic; Minoan (Cretan sea-people) replaces them.
UPDATE provinces SET culture_id = 'minoan' WHERE culture_id = 'pelasger';
