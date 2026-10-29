-- Wine ceased being food in 224. Preserve the strict food ledger and return
-- wine only where ShipDeparturePort proves a currently owned, active port.
DO $$
DECLARE u RECORD; port UUID;
BEGIN
 FOR u IN SELECT * FROM units WHERE provision_goods ? 'wine' FOR UPDATE LOOP
  port := NULL;
  IF u.status='freighting' THEN
   SELECT s.id INTO port FROM units ship
   LEFT JOIN LATERAL (SELECT id,status,dest_id FROM transports WHERE ship_unit_id=ship.id AND (ship.freight_bound_at IS NULL OR created_at>=ship.freight_bound_at) ORDER BY created_at DESC,id DESC LIMIT 1) last ON true
   JOIN settlements s ON s.id=CASE WHEN last.id IS NULL THEN ship.settlement_id WHEN last.status='delivered' THEN last.dest_id END AND s.owner_id=ship.owner_id AND s.state='active'
   WHERE ship.id=u.id AND NOT EXISTS(SELECT 1 FROM transports t WHERE t.ship_unit_id=ship.id AND t.status='in_transit');
  ELSIF u.status='garrison' AND u.category='naval' THEN
   SELECT s.id INTO port FROM settlements s JOIN provinces p ON p.id=s.province_id AND p.world_id=u.world_id
   WHERE s.id=u.settlement_id AND s.world_id=u.world_id AND s.owner_id=u.owner_id AND s.state='active'
   AND (u.q IS NULL OR u.q=p.map_q) AND (u.r IS NULL OR u.r=p.map_r);
  END IF;
  IF port IS NOT NULL AND COALESCE((u.provision_goods->>'wine')::float8,0)>0 THEN
   PERFORM credit_settlement_good(port,'wine',(u.provision_goods->>'wine')::float8,'retired_wine_provisions');
  END IF;
  UPDATE units SET provision_goods=provision_goods-'wine',
   provisions=COALESCE((provision_goods->>'grain')::float8,0)+COALESCE((provision_goods->>'fish')::float8,0)+COALESCE((provision_goods->>'oil')::float8,0),updated_at=now()
   WHERE id=u.id;
 END LOOP;
END $$;
