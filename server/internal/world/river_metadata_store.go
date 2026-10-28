package world

import (
 "context"
 "fmt"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// CompleteRiverMetadata atomically fills only missing static geography, before
// HTTP/workers start. Existing saved masks never change on startup or FOV reads.
func CompleteRiverMetadata(ctx context.Context,pool *pgxpool.Pool) error {
 tx,err:=pool.Begin(ctx);if err!=nil {return err};defer tx.Rollback(ctx)
 if _,err:=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(221,'map_tiles'::regclass::oid::integer)`);err!=nil {return err}
 rows,err:=tx.Query(ctx,`SELECT DISTINCT world_id FROM map_tiles WHERE terrain IN ('river','river_ford') AND river_edges IS NULL ORDER BY world_id`);if err!=nil {return err}
 var worlds []uuid.UUID
 for rows.Next(){var id uuid.UUID;if err:=rows.Scan(&id);err!=nil {rows.Close();return err};worlds=append(worlds,id)}
 rows.Close();if err:=rows.Err();err!=nil {return err}
 for _,id:=range worlds {
  var width,height int
  if err:=tx.QueryRow(ctx,`SELECT map_width,map_height FROM worlds WHERE id=$1 FOR UPDATE`,id).Scan(&width,&height);err!=nil {return err}
  rows,err:=tx.Query(ctx,`SELECT q,r,terrain FROM map_tiles WHERE world_id=$1 ORDER BY q,r FOR UPDATE`,id);if err!=nil {return err}
  var tiles []MapTile
  for rows.Next(){var tile MapTile;if err:=rows.Scan(&tile.Q,&tile.R,&tile.Terrain);err!=nil {rows.Close();return err};tiles=append(tiles,tile)}
  rows.Close();if err:=rows.Err();err!=nil {return err}
  PopulateRiverMetadata(tiles,width,height)
  for _,tile:=range tiles {
   if !tile.RiverMetadataReady {continue}
   if _,err:=tx.Exec(ctx,`UPDATE map_tiles SET river_edges=$4,river_upstream_edges=$5,river_downstream_edges=$6,river_flow_known=$7 WHERE world_id=$1 AND q=$2 AND r=$3 AND river_edges IS NULL`,id,tile.Q,tile.R,tile.RiverEdgesMask,tile.RiverUpstreamMask,tile.RiverDownstreamMask,tile.RiverFlowKnown);err!=nil {return fmt.Errorf("save river metadata: %w",err)}
  }
 }
 return tx.Commit(ctx)
}
