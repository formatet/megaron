-- Vägar (megaron_plan_vagar.md V1): en väg är en KANT mellan två grannhexar.
-- Kanten lagras oordnad i kanonisk ordning (q1,r1) < (q2,r2) (F6) så (A,B) och
-- (B,A) är samma rad. En rad finns först när kanten är KLAR (F4). Vägar står
-- för evigt; built_by_wanax är bara visning och släpps om kontot raderas.
CREATE TABLE road_edges (
    world_id        UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    q1              INT  NOT NULL,
    r1              INT  NOT NULL,
    q2              INT  NOT NULL,
    r2              INT  NOT NULL,
    built_by_wanax  UUID REFERENCES players(id) ON DELETE SET NULL,
    built_tick      INT  NOT NULL,
    PRIMARY KEY (world_id, q1, r1, q2, r2),
    CONSTRAINT road_edges_canonical CHECK (q1 < q2 OR (q1 = q2 AND r1 < r2)),
    -- axiellt hexavstånd = 1 (de sex grannriktningarna)
    CONSTRAINT road_edges_neighbours CHECK (
        (q2 - q1, r2 - r1) IN ((1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1))
    )
);
