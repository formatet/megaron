-- Migration 061: silver_mine — dedikerad gruvbyggnad för silverproduktion.
-- Koppar och tenn förblir gatade av "mine"; silver byter till "silver_mine"
-- så att spelaren måste välja vilken resurs en gruva exploaterar.
-- BuildingSilverMine-konstanten och BuildingSpecs-posten är redan lagda i koden.
UPDATE production_rules SET building_type = 'silver_mine' WHERE good_key = 'silver';
