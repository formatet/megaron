-- Elite infantry requires foundry + bronze; cult_level controls temple spending tier.

ALTER TABLE settlements
    ADD COLUMN elite_infantry INT NOT NULL DEFAULT 0,
    ADD COLUMN cult_level TEXT NOT NULL DEFAULT 'enkel'
        CHECK (cult_level IN ('forsummad', 'enkel', 'vardig', 'praktfull', 'overdadig'));

ALTER TABLE marching_armies
    ADD COLUMN elite_infantry INT NOT NULL DEFAULT 0;
