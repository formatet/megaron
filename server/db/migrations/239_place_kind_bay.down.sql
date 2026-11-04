-- Lossy: bay rows return to 'strait' only where no strait row holds the anchor;
-- the others (and any unmapped bay) are deleted by the CHECK restore below.
UPDATE place_names b SET kind='strait'
 WHERE b.kind='bay'
   AND NOT EXISTS (SELECT 1 FROM place_names s WHERE s.world_id=b.world_id AND s.kind='strait' AND s.anchor_q=b.anchor_q AND s.anchor_r=b.anchor_r);
DELETE FROM place_names WHERE kind='bay';
ALTER TABLE place_names DROP CONSTRAINT place_names_kind_check;
ALTER TABLE place_names ADD CONSTRAINT place_names_kind_check CHECK (kind IN ('river','massif','desert','forest','lake','island','strait','cape','plain','hills','delta'));
