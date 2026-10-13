WITH hosts AS (
		     SELECT hu.q, hu.r
		     FROM units hu
		     JOIN founder_phase fp ON fp.host_unit_id = hu.id AND fp.active
		     WHERE hu.world_id = $1 AND hu.q IS NOT NULL AND hu.r IS NOT NULL
		 ),
		 -- Per-landmass load: every occupied tile (settled province or active
		 -- host) resolved to its map_tiles.landmass_id and counted. Tiles on a
		 -- world generated before migration 124 (or the sea rows themselves)
		 -- have landmass_id NULL, so they are excluded here on purpose — an
		 -- old world simply gets no rows in this CTE and every candidate below
		 -- falls through tier-1 with COALESCE(...,0), i.e. no landmass signal,
		 -- deferring to the ore bias / RANDOM() tiers exactly as before Slice 2.
		 landmass_load AS (
		     SELECT lt.landmass_id, count(*) AS count
		     FROM (
		         SELECT map_q AS q, map_r AS r FROM provinces WHERE world_id = $1
		         UNION ALL
		         SELECT q, r FROM hosts
		     ) occ
		     JOIN map_tiles lt ON lt.world_id = $1 AND lt.q = occ.q AND lt.r = occ.r
		     WHERE lt.landmass_id IS NOT NULL
		     GROUP BY lt.landmass_id
		 ),
		 -- Ore tiles materialised once (a few dozen rows on a 230² map) instead
		 -- of re-scanning all of map_tiles per candidate below — the EXISTS
		 -- against map_tiles nb was a nested-loop re-scan of ~53k rows for each
		 -- of ~11k eligible candidates (~49s on a fresh 230² world, blowing the
		 -- 30s request timeout). Found 2026-08-07 the first time a join ran
		 -- against a truly empty full-size world. MATERIALIZED is required —
		 -- Postgres 12+ inlines plain CTEs, which silently undoes this fix.
		 copper_tiles AS MATERIALIZED (
		     SELECT q, r FROM map_tiles WHERE world_id = $1 AND copper_deposit
		 ),
		 tin_tiles AS MATERIALIZED (
		     SELECT q, r FROM map_tiles WHERE world_id = $1 AND tin_deposit
		 ),
		 silver_tiles AS MATERIALIZED (
		     SELECT q, r FROM map_tiles WHERE world_id = $1 AND COALESCE(silver_deposit, false)
		 ),
		 -- Landmasses with at least one timber hex (megaron_plan_byggkostnader
		 -- steg 4): every building costs timber and a new city starts with none, so
		 -- a landmass without forest is a dead end. Stone needs no rule — fieldstone
		 -- (mig 156) is on every land hex. The timber terrains are read from
		 -- production_rules, not listed here. MATERIALIZED for the same reason as
		 -- the ore CTEs above: an inlined CTE is re-evaluated per candidate.
		 viable_landmasses AS MATERIALIZED (
		     SELECT DISTINCT vt.landmass_id
		     FROM map_tiles vt
		     WHERE vt.world_id = $1 AND vt.landmass_id IS NOT NULL
		       AND vt.terrain IN (SELECT terrain_type FROM production_rules
		                          WHERE good_key = 'timber' AND building_type IS NULL
		                            AND terrain_type IS NOT NULL)
		 )
		 SELECT mt.q, mt.r, mt.terrain,
		        mt.copper_deposit, mt.tin_deposit,
		        COALESCE(mt.silver_deposit, false), COALESCE(mt.cedar_deposit, false),
		        COALESCE(mt.coastal, false)
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 LEFT JOIN landmass_load ll ON ll.landmass_id = mt.landmass_id
		 WHERE mt.world_id = $1
		   AND p.id IS NULL
		   -- A rising Wanax is a refugee: never back on the landmass that fell ($4).
		   AND ($4::int IS NULL OR mt.landmass_id IS DISTINCT FROM $4::int)
		   AND mt.terrain NOT IN ('coastal_sea','deep_sea','river','river_ford','mountain_limestone','mountain_red','semi_desert')
		   -- A world generated before mig 124 has landmass_id NULL everywhere; let
		   -- those candidates through rather than declare the whole world full.
		   AND (mt.landmass_id IS NULL OR mt.landmass_id IN (SELECT landmass_id FROM viable_landmasses))
		   -- The "<= 4" below is the HOST spawn spacing, not the founding minimum:
		   -- it deliberately does NOT follow province.minSettlementCentreDistance
		   -- (lowered to 3 by megaron_plan_delad_catchment.md). Do not "fix" it.
		   -- Keep clear of settled ground …
		   AND NOT EXISTS (
		       SELECT 1 FROM provinces p2
		       WHERE p2.world_id = $1
		         AND (ABS(mt.q - p2.map_q) + ABS(mt.r - p2.map_r) +
		              ABS((mt.q + mt.r) - (p2.map_q + p2.map_r))) / 2 <= 4
		   )
		   -- … and of other hosts, by the same measure.
		   AND NOT EXISTS (
		       SELECT 1 FROM hosts h
		       WHERE (ABS(mt.q - h.q) + ABS(mt.r - h.r) +
		              ABS((mt.q + mt.r) - (h.q + h.r))) / 2 <= 4
		   )
		   -- NOTE: the old "starter catchment must hold a grain tile" filter is
		   -- deliberately gone. It was a self-sufficiency invariant for a capital
		   -- born where it lands; a host carries four months of rations and is
		   -- meant to go looking for its site. Pre-picking fertile ground would
		   -- answer the question the founder phase exists to ask. The grain check
		   -- lives in the founding forecast instead
		   -- (temenos_nomadic_host_plan.md §Spawn, §Platsprognos).
		 ORDER BY
		   -- 1. Landmass balance: candidates on a landmass_id sort ahead of NULL
		   --    ones (old world, no migration-124 data), then by ascending load
		   --    (fewest settlements+hosts on that landmass first) so the next
		   --    joiner fills the emptiest continent instead of the emptiest
		   --    hemisphere.
		   (mt.landmass_id IS NULL) ASC,
		   COALESCE(ll.count, 0) ASC,
		   -- 2. Metal-catchment bias (tiebreak within the winning landmass):
		   --    west tiles that have a copper deposit within the future catchment
		   --    ring rank ahead of those that do not; east tiles prefer tin. This
		   --    ensures the first joiners land on ore-catchment tiles so they mine
		   --    from turn 1 — the self-sufficiency invariant is preserved because
		   --    the viability filters above still gate every candidate tile.
		   --    When no ore-catchment tile is eligible the bias is 0 for all and
		   --    we fall back to RANDOM() as before. Distance uses the same sum/2
		   --    axial hex-distance formula as the clearance checks above
		   --    (equivalent to hexgrid.Distance's max() formula); radius is
		   --    hexgrid.CatchmentRadius ($3, P1), excludes the tile itself (dist 0).
		   --
		   --    Silver scores 1 against the hemisphere ore's 2, so ore still wins
		   --    a straight fight and a silver tile only wins over ground with no
		   --    metal at all. Before this term nothing steered a settler towards
		   --    silver at all, and the drift world shows what that costs: 8
		   --    copper hexes reached 4 of 8 cities' catchments, 4 silver hexes
		   --    reached 1. Measured on fresh 60x60 worlds (10 joiners, four
		   --    runs) the term moves silver from 3-4 hosts to a steady 4
		   --    (megaron_silvergeografin.md, 2026-08-27) — small here because
		   --    tier 1 above already scatters joiners across landmasses, and
		   --    a 60x60 landmass rarely holds more than one metal. It matters
		   --    where landmasses are large enough for this tiebreak to actually
		   --    choose between many candidate tiles.
		   (CASE
		     WHEN mt.q <= $2 THEN (
		       EXISTS (
		         SELECT 1 FROM copper_tiles nb
		         WHERE (ABS(nb.q - mt.q) + ABS(nb.r - mt.r) + ABS((nb.q + nb.r) - (mt.q + mt.r)))
		               BETWEEN 2 AND 2 * $3::int
		       )::int
		     )
		     ELSE (
		       EXISTS (
		         SELECT 1 FROM tin_tiles nb
		         WHERE (ABS(nb.q - mt.q) + ABS(nb.r - mt.r) + ABS((nb.q + nb.r) - (mt.q + mt.r)))
		               BETWEEN 2 AND 2 * $3::int
		       )::int
		     )
		   END * 2 + EXISTS (
		     SELECT 1 FROM silver_tiles nb
		     WHERE (ABS(nb.q - mt.q) + ABS(nb.r - mt.r) + ABS((nb.q + nb.r) - (mt.q + mt.r)))
		           BETWEEN 2 AND 2 * $3::int
		   )::int) DESC,
		   RANDOM()
		 LIMIT 1
