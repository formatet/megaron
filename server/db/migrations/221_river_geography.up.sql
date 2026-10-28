-- Static river geography is written with new tiles, or once from the complete
-- saved map by world.CompleteRiverMetadata before the server starts serving.
ALTER TABLE map_tiles
 ADD COLUMN river_edges SMALLINT,
 ADD COLUMN river_upstream_edges SMALLINT,
 ADD COLUMN river_downstream_edges SMALLINT,
 ADD COLUMN river_flow_known BOOLEAN,
 ADD CONSTRAINT map_river_metadata CHECK (
  (river_edges IS NULL AND river_upstream_edges IS NULL AND river_downstream_edges IS NULL AND river_flow_known IS NULL)
  OR
  (terrain IN ('river','river_ford') AND river_edges BETWEEN 0 AND 63
   AND river_upstream_edges IS NOT NULL AND river_upstream_edges BETWEEN 0 AND 63
   AND river_downstream_edges IS NOT NULL AND river_downstream_edges BETWEEN 0 AND 63
   AND river_flow_known IS NOT NULL AND river_edges IS NOT NULL
   AND (river_upstream_edges & river_edges)=river_upstream_edges
   AND (river_downstream_edges & river_edges)=river_downstream_edges
   AND (river_upstream_edges & river_downstream_edges)=0
   AND (river_flow_known OR (river_upstream_edges=0 AND river_downstream_edges=0)))
 );
CREATE INDEX map_river_metadata_missing ON map_tiles(world_id)
 WHERE terrain IN ('river','river_ford') AND river_edges IS NULL;
