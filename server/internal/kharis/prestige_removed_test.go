package kharis

import (
	"context"
	"testing"
)

// World prestige was written every kharis tick and read by nothing (slice 17).
// Migration 188 drops the column; the tick no longer writes to worlds.
func TestWorldPrestigeColumnRemoved(t *testing.T) {
	pool := testPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_name = 'worlds' AND column_name = 'prestige'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("worlds.prestige still exists")
	}
}
