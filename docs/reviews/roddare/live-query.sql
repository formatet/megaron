BEGIN READ ONLY;
SELECT w.id AS world_id,w.current_tick,w.status AS world_status,now() AS observed_at,
u.id,u.type,u.status,u.crew,u.size,u.provisions,u.provision_goods,u.q,u.r,u.target_q,u.target_r,
u.depart_tick,u.arrive_tick,u.march_intent,u.home_settlement_id,u.cargo_unit_id,u.march_route
FROM units u JOIN worlds w ON w.id=u.world_id WHERE u.category='naval' AND u.status <> 'disbanded'
ORDER BY w.id,u.id;
COMMIT;
