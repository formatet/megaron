package religion

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the DB surface the index needs — satisfied by *pgxpool.Pool and
// pgx.Tx alike. Same shape as economy.DBTX (recompute.go:59); consumer-side
// interface per the G1 rule.
type Execer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// FixedDivineValues is the gods' price list per unit of a good (Timothy
// 2026-10-31: a fixed table, not the world's scarcity). The numbers are the
// alfa world's scarcity values on 2026-10-31 rounded up to whole numbers, so
// the switch keeps the balance players already met. Codex rites.md prints it.
var FixedDivineValues = map[string]float64{
	"grain": 1, "fish": 2, "livestock": 2, "timber": 2, "silver": 2,
	"stone": 3, "wine": 3, "oil": 3, "copper": 3, "tin": 3, "cedar": 3,
	"incense": 12, "bronze": 33,
}

// LoadDivineValues returns the divine price list as good_key→value. Every
// consumer — prayers, temple tithe, the cult's flow, the views — reads it here.
// The world and DB are not consulted; the signature stays so callers need not change.
func LoadDivineValues(ctx context.Context, db Execer, worldID uuid.UUID) (map[string]float64, error) {
	values := make(map[string]float64, len(FixedDivineValues))
	for k, v := range FixedDivineValues {
		values[k] = v
	}
	return values, nil
}

// LaborValueUnit labels the internal measure in persistent event payloads; the temple presents offering values.
const LaborValueUnit = "worker_ticks"

// Version 2 offerings were valued by the world-scarcity index; version 3 by
// FixedDivineValues. Recorded events keep the version they were valued under.
const LaborValuationVersion = 3

// LoadLaborValues is the shared reference used by the index and both cult consumers.
func LoadLaborValues(ctx context.Context, db Execer) (map[string]float64, error) {
	rows, err := db.Query(ctx, `SELECT key,labor_value FROM goods WHERE labor_value IS NOT NULL AND status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]float64{}
	for rows.Next() {
		var key string
		var value float64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}
