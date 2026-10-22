-- Run immediately before deployment, using psql -X -v ON_ERROR_STOP=1.
-- No rows means no materially positive old onboard provisions outside port.
-- Residual floating-point crumbs <=1e-9 are not a funded meal.
-- Do not top up, multiply food or recall ships through this query.
BEGIN READ ONLY;
SELECT now() AS observed_at,w.id AS world_id,w.status AS world_status,w.current_tick,
 u.id,u.name,u.type,u.status,u.crew,u.provisions,u.provision_goods,
 u.q,u.r,u.target_q,u.target_r,u.depart_tick,u.arrive_tick,u.march_intent,
 u.home_settlement_id,u.settlement_id,
 (SELECT jsonb_agg(jsonb_build_object('id',t.id,'kind',t.kind,
   'departed_tick',t.departed_tick,'due_tick',t.due_tick,'origin_id',t.origin_id,'dest_id',t.dest_id))
  FROM transports t WHERE t.ship_unit_id=u.id AND t.status='in_transit') AS active_freight
FROM units u JOIN worlds w ON w.id=u.world_id
WHERE u.category='naval' AND u.status IN ('marching','positioned','freighting')
 AND u.provisions > 1e-9
ORDER BY w.id,u.id;
COMMIT;
