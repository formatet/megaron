-- Cities in memory as they were seen (megaron_plan_vagbeskrivningar §S0,
-- Timothy 2026-10-28 20:50/20:53). A city on a remembered hex is shown as the
-- player last saw it, not as it is now. Written by province.RecordSeenSettlements
-- at every memory write and live sighting; read for memory-only hexes.
-- population is kept only to derive the visible size tier.
CREATE TABLE player_seen_settlements (
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    q             INT NOT NULL,
    r             INT NOT NULL,
    name          TEXT NOT NULL,
    owner_id      UUID,
    owner_name    TEXT NOT NULL DEFAULT '',
    is_capital    BOOLEAN NOT NULL DEFAULT false,
    state         TEXT NOT NULL,
    wall_level    INT NOT NULL DEFAULT 0,
    population    INT NOT NULL DEFAULT 0,
    seen_tick     INT NOT NULL,
    PRIMARY KEY (player_id, settlement_id)
);
CREATE INDEX player_seen_settlements_hex ON player_seen_settlements (world_id, player_id, q, r);

-- Transition: every city standing on a remembered hex today (scouted tiles,
-- legacy scouted provinces, messenger contacts — loadRememberedTiles' union) is snapshotted
-- as it is now — exactly what the player sees before this migration.
INSERT INTO player_seen_settlements
       (world_id, player_id, settlement_id, q, r, name, owner_id, owner_name,
        is_capital, state, wall_level, population, seen_tick)
SELECT s.world_id, pst.player_id, s.id, p.map_q, p.map_r, s.name, s.owner_id,
       COALESCE(pl.wanax_name, pl.username, ''), s.is_capital, s.state,
       s.wall_level, s.population, w.current_tick
  FROM settlements s
  JOIN provinces p ON p.id = s.province_id
  JOIN worlds w ON w.id = s.world_id
  JOIN (SELECT world_id, player_id, q, r FROM player_scouted_tiles
        UNION
        SELECT sp.world_id, sp.player_id, pp.map_q, pp.map_r
          FROM player_scouted_provinces sp JOIN provinces pp ON pp.id = sp.province_id
        UNION
        SELECT m.world_id, m.sender_id, mp.map_q, mp.map_r
          FROM messengers m
          JOIN settlements ms ON ms.id = m.destination_id
          JOIN provinces mp ON mp.id = ms.province_id
         WHERE NOT m.withdrawn AND m.status IN ('delivered', 'returning', 'arrived')
       ) pst
    ON pst.world_id = s.world_id AND pst.q = p.map_q AND pst.r = p.map_r
  LEFT JOIN players pl ON pl.id = s.owner_id
 WHERE s.state != 'sunk'
ON CONFLICT DO NOTHING;
