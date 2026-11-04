-- Bays are a place kind of their own (sund-och-vikar, Timothy 2026-11-02). Every
-- strait named before this was a one-hex bay (docs/reviews/sund-och-vikar/README.md),
-- so the rows move to 'bay' with name and claim intact. Real straits are new places.
ALTER TABLE place_names DROP CONSTRAINT place_names_kind_check;
ALTER TABLE place_names ADD CONSTRAINT place_names_kind_check CHECK (kind IN ('river','massif','desert','forest','lake','island','strait','bay','cape','plain','hills','delta'));
UPDATE place_names SET kind='bay' WHERE kind='strait';
