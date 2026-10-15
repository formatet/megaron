-- Vägar V2 (megaron_plan_vagar.md): ett vägbygge är en rad tills kanten är klar.
-- Stenen dras från hemstaden när ordern ges och går som en fysisk karavan
-- (transports.kind='road_stone', dest_id NULL, dest_q/dest_r = kantens "huvud");
-- ordern till armén går med bud. Bygget startar när BÅDA är framme. Avbruten
-- kant återbetalas inte (B3). Raden är också det som gör en fastnad väg synlig
-- för ägaren (state='waiting').
CREATE TABLE road_sites (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id           UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id           UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    unit_id            UUID        REFERENCES units(id) ON DELETE SET NULL,
    from_q             INT         NOT NULL,
    from_r             INT         NOT NULL,
    to_q               INT         NOT NULL,
    to_r               INT         NOT NULL,
    stone              DOUBLE PRECISION NOT NULL CHECK (stone > 0),
    home_settlement_id UUID        REFERENCES settlements(id) ON DELETE SET NULL,
    stone_transport_id UUID        REFERENCES transports(id) ON DELETE SET NULL,
    stone_arrived      BOOLEAN     NOT NULL DEFAULT false,
    army_arrived       BOOLEAN     NOT NULL DEFAULT false,
    state              TEXT        NOT NULL DEFAULT 'waiting'
                                   CHECK (state IN ('waiting', 'building', 'done', 'cancelled')),
    build_due_tick     INT,
    created_tick       INT         NOT NULL,
    CONSTRAINT road_sites_neighbours CHECK (
        (to_q - from_q, to_r - from_r) IN ((1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1))
    )
);
-- The owner's list of active sites and a unit's active site are the two reads.
CREATE INDEX road_sites_active_owner ON road_sites (world_id, owner_id) WHERE state IN ('waiting', 'building');
CREATE INDEX road_sites_active_unit ON road_sites (unit_id) WHERE state IN ('waiting', 'building');
CREATE INDEX road_sites_stone_transport ON road_sites (stone_transport_id) WHERE stone_transport_id IS NOT NULL;
