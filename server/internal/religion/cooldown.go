package religion

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"math"
)

type PrayerDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// MaxGiftFactor caps how much a larger gift can stretch a prayer (alpha:
// generous, but nothing absurd). It bounds both the length and the rest.
const MaxGiftFactor = 5.0

// GiftFactor is r, the size of a gift against the traditional one: worth over
// baseline, never below 1 (a gift under the baseline is refused) and never
// above MaxGiftFactor. Without a reckoning (baseline 0) the gift is traditional.
func GiftFactor(worth, baseline float64) float64 {
	if baseline <= 0 || worth <= baseline {
		return 1
	}
	return min(MaxGiftFactor, worth/baseline)
}

// DurationTicks is how long a lasting prayer holds: round(base x r). r = 1
// gives exactly the base; r is capped at MaxGiftFactor.
func DurationTicks(base int, r float64) int {
	return int(math.Round(float64(base) * min(MaxGiftFactor, max(1, r))))
}

// RestTicks is how long the temple rests after an answer: the prayer's
// cooldown grows with the gift and shrinks with the temple's level.
func RestTicks(cooldown int, r float64, level int) int {
	if cooldown <= 0 || level < 1 {
		return 0
	}
	return int(math.Ceil(float64(cooldown) * min(MaxGiftFactor, max(1, r)) / float64(level)))
}

// TempleRestRemaining is the ticks a temple still rests (0 = ready). The row
// is counted down by economy.AdvanceTempleRest, only while fully staffed.
func TempleRestRemaining(ctx context.Context, db PrayerDB, building uuid.UUID) (int, error) {
	var n int
	if err := db.QueryRow(ctx, `SELECT COALESCE((SELECT remaining_ticks FROM temple_rest WHERE building_id=$1),0)`, building).Scan(&n); err != nil {
		return 0, fmt.Errorf("read temple rest: %w", err)
	}
	return n, nil
}

// SetTempleRest is called only after a successful answer, in the same TX as
// the effect, sacrifice and outcome event. last_tick = the cast tick so the
// same tick's count-down does not shorten it.
func SetTempleRest(ctx context.Context, db PrayerDB, building uuid.UUID, ticks, nowTick int) error {
	if ticks <= 0 {
		return nil
	}
	_, err := db.Exec(ctx, `INSERT INTO temple_rest(building_id,remaining_ticks,last_tick) VALUES($1,$2,$3) ON CONFLICT(building_id) DO UPDATE SET remaining_ticks=EXCLUDED.remaining_ticks,last_tick=EXCLUDED.last_tick`, building, ticks, nowTick)
	return err
}

// PrayerActiveRemaining is shared by the cast guard and the prayer catalogue.
// The owner stamp prevents the new city's owner inheriting a defensive gift.
func PrayerActiveRemaining(ctx context.Context, db PrayerDB, settlement, player uuid.UUID, effect string, nowTick int) (int, error) {
	if effect == EffectNextLandDeparture || effect == EffectNextShipDeparture {
		pending, err := DeparturePending(ctx, db, settlement, player, effect)
		if pending {
			return 1, err
		}
		return 0, err
	}
	var until int
	err := db.QueryRow(ctx, `SELECT CASE
 WHEN $3='garrison_defense' AND owner_id=$2 AND garrison_defense_owner_id=$2 THEN COALESCE(garrison_defense_until_tick,0)
 WHEN $3='harvest_blessing' THEN COALESCE(harvest_blessing_until_tick,0)
 ELSE 0 END FROM settlements WHERE id=$1`, settlement, player, effect).Scan(&until)
	if err != nil {
		return 0, err
	}
	return max(0, until-nowTick), nil
}
