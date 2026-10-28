DROP INDEX map_river_metadata_missing;
ALTER TABLE map_tiles DROP CONSTRAINT map_river_metadata,
 DROP COLUMN river_edges, DROP COLUMN river_upstream_edges,
 DROP COLUMN river_downstream_edges, DROP COLUMN river_flow_known;
