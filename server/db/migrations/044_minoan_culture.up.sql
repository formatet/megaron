-- Migration 044: Pelasger → Minoan culture rename
-- culture_id lives on settlements (provinces lost it in mig 005).
UPDATE settlements SET culture_id = 'minoan' WHERE culture_id = 'pelasger';
