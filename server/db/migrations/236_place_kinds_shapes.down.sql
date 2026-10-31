DELETE FROM place_names WHERE kind IN ('island','strait','cape','plain','hills','delta');
ALTER TABLE place_names DROP CONSTRAINT place_names_kind_check;
ALTER TABLE place_names ADD CONSTRAINT place_names_kind_check CHECK (kind IN ('river','massif','desert','forest','lake'));
