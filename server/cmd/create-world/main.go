package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/world"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	// Archive old worlds
	tx.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`)

	// Create world
	id := uuid.New()
	seed := time.Now().UnixNano()
	width, height := 56, 40

	if _, err := tx.Exec(ctx,
		`INSERT INTO worlds (id, name, map_seed, map_width, map_height, status, state)
		 VALUES ($1, $2, $3, $4, $5, 'active', 'active')`,
		id, "Megaron fresh reseed", seed, width, height,
	); err != nil {
		log.Fatal(err)
	}

	// Generate map
	tiles, effSeed := world.GenerateMap(id, seed, width, height)

	if effSeed != seed {
		if _, err := tx.Exec(ctx, `UPDATE worlds SET map_seed = $1 WHERE id = $2`, effSeed, id); err != nil {
			log.Fatal(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}

	// Store tiles (outside transaction)
	batch := &pgx.Batch{}
	for _, t := range tiles {
		batch.Queue(
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal, fertility, mineral, copper_deposit, tin_deposit, silver_deposit, cedar_deposit)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, t.Q, t.R, string(t.Terrain), t.Coastal, t.Fertility, t.Mineral, t.CopperDeposit, t.TinDeposit, t.SilverDeposit, t.CedarDeposit,
		)
	}
	br := pool.SendBatch(ctx, batch)
	br.Close()

	// Count tin
	var tinCount int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM map_tiles WHERE world_id = $1 AND tin_deposit", id).Scan(&tinCount)

	fmt.Printf("✓ World created: %s\n", id)
	fmt.Printf("✓ Tin deposits: %d\n", tinCount)
	if tinCount < 2 {
		fmt.Printf("⚠️  WARNING: Tin < 2 — reseed again!\n")
		os.Exit(1)
	}
}
