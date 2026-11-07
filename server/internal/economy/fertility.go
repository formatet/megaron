package economy

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
)

// Potnia's fertility (bönesystemet v3 steg 5a): while the blessing lasts a covered city's
// population grows, and its herd calves, half as fast again. The blessing is a row in
// prayer_blessings (scope city or realm); economy may not import religion, so it names
// the effect itself — a handlers test pins FertilityEffect to religion.EffectBlessedFertility.
const (
	FertilityEffect       = "blessed_fertility"
	FertilityGrowthFactor = 1.5
)

// fertilityCoversSQL is the SQL boolean "a live fertility blessing covers settlement <s> at <tick>":
// the owner's own realm blessing, or a city blessing laid on this very city. Owner is checked,
// so another Wanax's blessing never covers it.
func fertilityCoversSQL(s, tickExpr string) string {
	return fmt.Sprintf(`EXISTS(SELECT 1 FROM prayer_blessings fb WHERE fb.effect='%s' AND fb.owner_id=%s.owner_id
   AND fb.from_tick<=%s AND fb.until_tick>%s AND (fb.scope='realm' OR (fb.scope='city' AND fb.settlement_id=%s.id)))`,
		FertilityEffect, s, tickExpr, tickExpr, s)
}

// fertilityFactorSQL is the growth multiplier of settlement s now (1 when not blessed).
func fertilityFactorSQL(s string) string {
	return fmt.Sprintf(`(CASE WHEN %s THEN %g ELSE 1 END)::float8`, fertilityCoversSQL(s, "current_world_tick()"), FertilityGrowthFactor)
}

// FertileBirths is the herd's calves under the blessing: half again, rounded down, never fewer.
func FertileBirths(born int) int {
	return max(born, int(math.Floor(float64(born)*FertilityGrowthFactor)))
}

// fertileAt says whether settlement sid is under a live fertility blessing at tick.
func fertileAt(ctx context.Context, tx Tx, sid uuid.UUID, tick int) (bool, error) {
	var yes bool
	err := tx.QueryRow(ctx, `SELECT `+fertilityCoversSQL("s", "$2::int")+` FROM settlements s WHERE s.id=$1`, sid, tick).Scan(&yes)
	return yes, err
}
