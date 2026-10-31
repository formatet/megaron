-- Terrain-shaped places are nameable (platstyper P1, Timothy 2026-10-31):
-- islands, straits, capes, plains, hills and river deltas.
ALTER TABLE place_names DROP CONSTRAINT place_names_kind_check;
ALTER TABLE place_names ADD CONSTRAINT place_names_kind_check CHECK (kind IN ('river','massif','desert','forest','lake','island','strait','cape','plain','hills','delta'));
