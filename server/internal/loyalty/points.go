package loyalty

import (
	"context"

	"github.com/google/uuid"
)

// ── Loyalty balance levers (Timothy 2026-07-11) ──────────────────────────────
// Loyalty is a slow continuous accumulator, modelled on kharis: a hidden
// loyalty_points value (1–100) crawls, and the integer loyalty (1–4) is DERIVED
// from bands. Loyalty is meant to be BUILT OVER WEEKS — no single daily tick can
// jump a settlement to max. Everything in this block is calibration: tunable, no
// invariants. Documented in temenos_balans_spakar.md §12.
const (
	// Band ceilings: points below Band{N}Ceil → loyalty N (≥ Band3Ceil → loyalty 4).
	LoyaltyBand1Ceil = 25.0 // points < 25      → loyalty 1 (near-revolt)
	LoyaltyBand2Ceil = 50.0 // 25 ≤ points < 50 → loyalty 2 (lojal)
	LoyaltyBand3Ceil = 75.0 // 50 ≤ points < 75 → loyalty 3 (hängiven)
	//                         points ≥ 75      → loyalty 4 (fanatisk)

	// Points are never exactly 0 (mirrors kharisFloor — loyalty always has a pulse).
	LoyaltyPointsFloor = 1.0
	LoyaltyPointsCap   = 100.0

	// Genesis starts: midpoint of the target band, so a fresh settlement sits
	// stable in the middle of its band rather than on an edge.
	LoyaltyStartCapital = 62.0 // metropolis starts loyalty 3 (Timothy 2026-07-11)
	LoyaltyStartColony  = 37.0 // colony starts loyalty 2

	// Crawl scaling: every integer loyalty "intent" that used to move loyalty by
	// ±1 directly (welfare ±1–2/day, gift +1, battle ±1, colony penalty −1/−3/−5,
	// decay −1) is now converted to a POINTS delta by these factors. With ~25-point
	// bands, a +2/day welfare gain ⇒ ~2 weeks per loyalty step. Gains crawl; losses
	// run faster so collapse outruns recovery (the accepted deserteringsspiral).
	LoyaltyPointsPerGain = 1.0
	LoyaltyPointsPerLoss = 1.5
)

// LoyaltyFromPoints derives the integer loyalty band (1–4) from points.
func LoyaltyFromPoints(p float64) int {
	switch {
	case p < LoyaltyBand1Ceil:
		return 1
	case p < LoyaltyBand2Ceil:
		return 2
	case p < LoyaltyBand3Ceil:
		return 3
	default:
		return 4
	}
}

// scaleDeltaToPoints converts an integer loyalty intent to a points delta,
// applying the asymmetric gain/loss scaling.
func scaleDeltaToPoints(delta int) float64 {
	if delta >= 0 {
		return float64(delta) * LoyaltyPointsPerGain
	}
	return float64(delta) * LoyaltyPointsPerLoss
}

// clampPoints bounds points to [LoyaltyPointsFloor, LoyaltyPointsCap].
func clampPoints(p float64) float64 {
	if p < LoyaltyPointsFloor {
		return LoyaltyPointsFloor
	}
	if p > LoyaltyPointsCap {
		return LoyaltyPointsCap
	}
	return p
}

// applyLoyaltyPointsDelta moves a settlement's loyalty_points by pointsDelta
// (already scaled), clamps to [floor, cap], and re-derives the integer loyalty
// band + trend — all in one atomic UPDATE. Shared by every loyalty sink
// (welfare/gift/battle via AppendLoyaltyEvent*, and decay). Works on a pool or a
// tx (loyaltyExecutor). Returns rows affected so callers can detect a no-op id.
func applyLoyaltyPointsDelta(ctx context.Context, db loyaltyExecutor, settlementID uuid.UUID, pointsDelta float64) (int64, error) {
	tag, err := db.Exec(ctx,
		`WITH np AS (
		     SELECT id, LEAST($6, GREATEST($5, loyalty_points + $2)) AS pts
		     FROM settlements WHERE id = $1
		 )
		 UPDATE settlements s
		 SET loyalty_points = np.pts,
		     loyalty = CASE
		         WHEN np.pts < $3 THEN 1
		         WHEN np.pts < $4 THEN 2
		         WHEN np.pts < $7 THEN 3
		         ELSE 4
		     END,
		     loyalty_trend = CASE
		         WHEN $2 > 0 THEN 'rising'
		         WHEN $2 < 0 THEN 'falling'
		         ELSE loyalty_trend
		     END
		 FROM np WHERE s.id = np.id`,
		settlementID, pointsDelta,
		LoyaltyBand1Ceil, LoyaltyBand2Ceil, LoyaltyPointsFloor, LoyaltyPointsCap, LoyaltyBand3Ceil,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
