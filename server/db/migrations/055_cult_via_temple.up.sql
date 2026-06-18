-- Migration 055: W4d — kult via tempel-labor. cult är en intern (icke-handlad)
-- vara producerad av tempel-byggnaden via labor-allokering; daglig tick
-- konverterar den till kharis. cult_level blir härledd ur kharis (ej spelar-satt).
ALTER TABLE goods DROP CONSTRAINT IF EXISTS goods_category_check;
ALTER TABLE goods ADD CONSTRAINT goods_category_check
    CHECK (category IN ('staple','strategic','prestige','bulk','precious','sacred'));

INSERT INTO goods (key, name, tier, category, base_value, weight)
VALUES ('cult', 'Cult', 'manufactured', 'sacred', 0, 0)
ON CONFLICT (key) DO NOTHING;

-- Tempel-labor → cult. Terräng-agnostisk, bygg-gatad på temple.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_coastal, requires_deposit)
VALUES (NULL, 'temple', 'cult', 0.05, FALSE, NULL);
